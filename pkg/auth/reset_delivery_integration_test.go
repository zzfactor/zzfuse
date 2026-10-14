//go:build integration

package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/medxops/trazr-otel/pkg/auth/storage"
	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

func TestPasswordResetSMTPAndAtomicPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_AUTH_RESET_DSN")
	if os.Getenv("TRAZR_IT") != "1" || dsn == "" {
		t.Skip("requires TRAZR_IT=1 and TEST_AUTH_RESET_DSN for a disposable database")
	}
	t.Setenv("BCRYPT_COST", "4")
	t.Setenv("TRAZR_PASSWORD_PEPPER", "")
	t.Setenv("PASSWORD_PEPPER", "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("auth_reset_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	store, err := pgstore.NewWithSchema(db, schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Users().EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.PasswordResets().EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	oldHash, err := HashPassword("OldPassword123")
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.Users().Create(ctx, storage.CreateUserParams{Email: "person@example.test", PasswordHash: oldHash, Name: "Fixture"})
	if err != nil {
		t.Fatal(err)
	}
	capture, roots := startSMTPCapture(t, "plain")
	sender, err := NewSMTPEmailSender(smtpCaptureConfig(capture, "plain", roots))
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := NewPasswordResetDelivery(sender, "https://example.test/reset-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := delivery.Request(ctx, store, "person@example.test"); err != nil {
		t.Fatal(err)
	}
	var privateMail string
	select {
	case privateMail = <-capture.messages:
	case <-time.After(time.Second):
		t.Fatal("no private SMTP reset delivery")
	}
	message, err := mail.ReadMessage(strings.NewReader(privateMail))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(message.Body))
	if err != nil {
		t.Fatal(err)
	}
	token := tokenInResetMail(t, string(body))
	var storedHash string
	var used sql.NullTime
	if err := db.QueryRowContext(ctx, "SELECT token_hash,used_at FROM "+schema+".auth_password_resets WHERE user_id=$1", user.ID).Scan(&storedHash, &used); err != nil {
		t.Fatal(err)
	}
	if storedHash == token || storedHash != hashToken(token) || used.Valid {
		t.Fatal("raw token persisted or prematurely consumed")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- resetPasswordWithStore(ctx, store, token, "NewPassword123") }()
	}
	wg.Wait()
	close(results)
	success, replay := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrInvalidResetToken) {
			replay++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || replay != 1 {
		t.Fatal("concurrent PostgreSQL reset consumed token more than once")
	}
	var passwordHash string
	if err := db.QueryRowContext(ctx, "SELECT password_hash FROM "+schema+".auth_users WHERE id=$1", user.ID).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if CheckPassword(passwordHash, "NewPassword123") != nil || CheckPassword(passwordHash, "OldPassword123") == nil {
		t.Fatal("new/old password behavior incorrect")
	}
	// Force the password update to fail after token claim, proving SQL rollback.
	second, err := generateResetToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PasswordResets().Create(ctx, user.ID, hashToken(second), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`CREATE FUNCTION %s.reject_password_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture rejects update'; END $$; CREATE TRIGGER reject_password_update BEFORE UPDATE OF password_hash ON %s.auth_users FOR EACH ROW EXECUTE FUNCTION %s.reject_password_update();`, schema, schema, schema)
	if _, err := db.ExecContext(ctx, trigger); err != nil {
		t.Fatal(err)
	}
	if err := resetPasswordWithStore(ctx, store, second, "AnotherPassword123"); err == nil {
		t.Fatal("failed password update accepted")
	}
	if err := db.QueryRowContext(ctx, "SELECT used_at FROM "+schema+".auth_password_resets WHERE token_hash=$1", hashToken(second)).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used.Valid {
		t.Fatal("failed SQL update consumed token")
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER reject_password_update ON "+schema+".auth_users"); err != nil {
		t.Fatal(err)
	}
	if err := resetPasswordWithStore(ctx, store, second, "AnotherPassword123"); err != nil {
		t.Fatal(err)
	}
	// SMTP failure invalidates the newly issued row, despite a canceled request.
	failed, _ := NewPasswordResetDelivery(emailSenderFunc(func(context.Context, string, string, string) error { return errors.New("fixture transport failure") }), "https://example.test/reset-password")
	if err := failed.Request(ctx, store, "person@example.test"); err == nil {
		t.Fatal("failed delivery accepted")
	}
	var active int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+schema+".auth_password_resets WHERE used_at IS NULL").Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatal("failed delivery left a usable token")
	}
	if err := delivery.Request(ctx, store, "missing@example.test"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-capture.messages:
		t.Fatal("missing account caused delivery")
	default:
	}
}
