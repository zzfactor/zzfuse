package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ErrEmailNotConfigured = errors.New("private email delivery is not configured")
	ErrEmailConfiguration = errors.New("invalid private email delivery configuration")
	ErrEmailDelivery      = errors.New("private email delivery failed")
)

// EmailSender is deliberately separate from reset-token storage so additional
// private transports can implement this interface without changing auth flows.
type EmailSender interface {
	Send(context.Context, string, string, string) error
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLSMode  string // starttls, tls (implicit), or plain (loopback only)
	Timeout  time.Duration
	RootCAs  *x509.CertPool
}

type smtpEmailSender struct {
	cfg  SMTPConfig
	from *mail.Address
}

func SMTPConfigFromEnv() (SMTPConfig, error) {
	cfg := SMTPConfig{Host: strings.TrimSpace(os.Getenv("TRAZR_SMTP_HOST")), Port: 587, Username: os.Getenv("TRAZR_SMTP_USERNAME"), Password: os.Getenv("TRAZR_SMTP_PASSWORD"), From: os.Getenv("TRAZR_SMTP_FROM"), TLSMode: strings.ToLower(strings.TrimSpace(os.Getenv("TRAZR_SMTP_TLS_MODE"))), Timeout: 10 * time.Second}
	if cfg.Host == "" {
		return cfg, ErrEmailNotConfigured
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = "starttls"
	}
	if v := os.Getenv("TRAZR_SMTP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("%w: SMTP port", ErrEmailConfiguration)
		}
		cfg.Port = n
	}
	if v := os.Getenv("TRAZR_SMTP_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("%w: SMTP timeout", ErrEmailConfiguration)
		}
		cfg.Timeout = d
	}
	if path := os.Getenv("TRAZR_SMTP_CA_FILE"); path != "" {
		certs, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("%w: SMTP CA file", ErrEmailConfiguration)
		}
		cfg.RootCAs, err = x509.SystemCertPool()
		if err != nil {
			cfg.RootCAs = x509.NewCertPool()
		}
		if !cfg.RootCAs.AppendCertsFromPEM(certs) {
			return cfg, fmt.Errorf("%w: SMTP CA certificates", ErrEmailConfiguration)
		}
	}
	if _, err := NewSMTPEmailSender(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func emailAddress(value string) (*mail.Address, error) {
	if strings.ContainsAny(value, "\r\n") {
		return nil, ErrEmailConfiguration
	}
	a, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil || a.Address == "" {
		return nil, ErrEmailConfiguration
	}
	return a, nil
}

func NewSMTPEmailSender(cfg SMTPConfig) (EmailSender, error) {
	if cfg.Host == "" {
		return nil, ErrEmailNotConfigured
	}
	if strings.ContainsAny(cfg.Host, "\r\n\t /?#") || cfg.Port < 1 || cfg.Port > 65535 || cfg.Timeout <= 0 || cfg.Timeout > time.Minute {
		return nil, ErrEmailConfiguration
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return nil, ErrEmailConfiguration
	}
	if cfg.TLSMode != "starttls" && cfg.TLSMode != "tls" && (cfg.TLSMode != "plain" || !loopbackHost(cfg.Host)) {
		return nil, ErrEmailConfiguration
	}
	from, err := emailAddress(cfg.From)
	if err != nil {
		return nil, ErrEmailConfiguration
	}
	if cfg.RootCAs != nil {
		cfg.RootCAs = cfg.RootCAs.Clone()
	}
	return &smtpEmailSender{cfg: cfg, from: from}, nil
}

// Send never logs addresses, credentials, message bodies, or server errors.
// SMTP failures are sanitized because servers can echo message/token content.
func (s *smtpEmailSender) Send(ctx context.Context, to, subject, body string) error {
	recipient, err := emailAddress(to)
	if err != nil || strings.ContainsAny(subject, "\r\n") {
		return ErrEmailDelivery
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return ErrEmailDelivery
	}
	var message bytes.Buffer
	fmt.Fprintf(&message, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@zzfuse.local>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", s.from.String(), recipient.String(), mime.QEncoding.Encode("UTF-8", subject), time.Now().UTC().Format(time.RFC1123Z), hex.EncodeToString(id[:]))
	qp := quotedprintable.NewWriter(&message)
	if _, err := qp.Write([]byte(body)); err != nil {
		return ErrEmailDelivery
	}
	if err := qp.Close(); err != nil {
		return ErrEmailDelivery
	}
	deadline := time.Now().Add(s.cfg.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	dialer := net.Dialer{Timeout: s.cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port)))
	if err != nil {
		return emailSendError(ctx)
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return ErrEmailDelivery
	}
	connection := conn
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	if s.cfg.TLSMode == "plain" {
		peer, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok || !peer.IP.IsLoopback() {
			return ErrEmailDelivery
		}
	}
	tlsConfig := &tls.Config{ServerName: s.cfg.Host, RootCAs: s.cfg.RootCAs, MinVersion: tls.VersionTLS12}
	if s.cfg.TLSMode == "tls" {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return emailSendError(ctx)
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return emailSendError(ctx)
	}
	defer client.Close()
	if s.cfg.TLSMode == "starttls" {
		if available, _ := client.Extension("STARTTLS"); !available {
			return ErrEmailDelivery
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return emailSendError(ctx)
		}
	}
	if s.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return emailSendError(ctx)
		}
	}
	if err := client.Mail(s.from.Address); err != nil {
		return emailSendError(ctx)
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return emailSendError(ctx)
	}
	writer, err := client.Data()
	if err != nil {
		return emailSendError(ctx)
	}
	if _, err := writer.Write(message.Bytes()); err != nil {
		_ = writer.Close()
		return emailSendError(ctx)
	}
	if err := writer.Close(); err != nil {
		return emailSendError(ctx)
	}
	// Acceptance after DATA is the delivery boundary. A failed QUIT must not
	// revoke a token that the server has already accepted for private delivery.
	_ = client.Quit()
	return nil
}
func emailSendError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Socket deadlines can fire just before the context timer records Err.
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return ErrEmailDelivery
}

// SendEmail uses explicit SMTP configuration and fails closed when disabled.
func SendEmail(ctx context.Context, to, subject, body string) error {
	cfg, err := SMTPConfigFromEnv()
	if err != nil {
		return err
	}
	sender, err := NewSMTPEmailSender(cfg)
	if err != nil {
		return err
	}
	return sender.Send(ctx, to, subject, body)
}
