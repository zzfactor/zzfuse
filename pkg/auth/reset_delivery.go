package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"database/sql"
	"github.com/medxops/trazr-otel/pkg/auth/storage"
	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

const PasswordResetLifetime = time.Hour

var ErrInvalidResetToken = errors.New("invalid or expired reset token")
var ErrInvalidResetPassword = errors.New("invalid new password length")

// PasswordResetDelivery holds an explicit private transport and frontend URL.
// Configuration does not depend on public request headers or supplied redirects.
type PasswordResetDelivery struct {
	Sender   EmailSender
	ResetURL *url.URL
}

func NewPasswordResetDelivery(sender EmailSender, resetURL string) (*PasswordResetDelivery, error) {
	if sender == nil {
		return nil, ErrEmailNotConfigured
	}
	u, err := url.Parse(resetURL)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && (u.Scheme != "http" || !loopbackHost(u.Hostname()))) {
		return nil, fmt.Errorf("%w: frontend reset URL", ErrEmailConfiguration)
	}
	return &PasswordResetDelivery{Sender: sender, ResetURL: u}, nil
}
func PasswordResetDeliveryFromEnv() (*PasswordResetDelivery, error) {
	cfg, err := SMTPConfigFromEnv()
	if err != nil {
		return nil, err
	}
	sender, err := NewSMTPEmailSender(cfg)
	if err != nil {
		return nil, err
	}
	return NewPasswordResetDelivery(sender, strings.TrimSpace(os.Getenv("TRAZR_PASSWORD_RESET_URL")))
}

// Request privately delivers to the stored account address. Callers must use the
// same public response for missing users and delivery/storage failures.
func (d *PasswordResetDelivery) Request(ctx context.Context, store storage.Store, email string) error {
	if d == nil || d.Sender == nil || d.ResetURL == nil || store == nil {
		return ErrEmailNotConfigured
	}
	user, err := store.Users().GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	if user == nil {
		return nil
	}
	token, err := generateResetToken()
	if err != nil {
		return err
	}
	hash := hashToken(token)
	if err := store.PasswordResets().Create(ctx, user.ID, hash, time.Now().UTC().Add(PasswordResetLifetime)); err != nil {
		return err
	}
	link := *d.ResetURL
	query := link.Query()
	query.Del("token")
	link.RawQuery = query.Encode()
	link.Fragment = url.Values{"token": []string{token}}.Encode()
	body := "A password reset was requested for your zzfuse account.\n\nOpen this link within one hour:\n" + link.String() + "\n\nIf you did not request this reset, you can ignore this email.\n"
	if err := d.Sender.Send(ctx, user.Email, "Reset your zzfuse password", body); err != nil {
		// Do not use a canceled request context for revocation. Bound cleanup to
		// avoid leaving a usable, undelivered token after request cancellation.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if revokeErr := store.PasswordResets().MarkUsed(cleanup, hash); revokeErr != nil {
			return errors.New("password reset delivery and token revocation failed")
		}
		return errors.New("password reset delivery failed")
	}
	return nil
}

// ResetPasswordWithToken consumes the token and updates the password in one
// transaction. A failed update leaves the token usable; replay cannot win twice.
func ResetPasswordWithToken(ctx context.Context, db *sql.DB, token, newPassword string) error {
	return resetPasswordWithStore(ctx, pgstore.New(db), token, newPassword)
}
func resetPasswordWithStore(ctx context.Context, store storage.Store, token, newPassword string) error {
	if strings.TrimSpace(token) == "" {
		return ErrInvalidResetToken
	}
	if len(newPassword) < 8 || len(newPassword)+len(getPasswordPepper()) > 72 {
		return ErrInvalidResetPassword
	}
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return store.WithTx(ctx, func(tx storage.Store) error {
		userID, err := tx.PasswordResets().Consume(ctx, hashToken(token))
		if err != nil {
			return err
		}
		if userID == 0 {
			return ErrInvalidResetToken
		}
		return tx.Users().UpdatePasswordHash(ctx, userID, passwordHash)
	})
}
