package auth

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

type smtpCapture struct {
	listener    net.Listener
	tlsConfig   *tls.Config
	mode        string
	messages    chan string
	mu          sync.Mutex
	connections []net.Conn
	wg          sync.WaitGroup
}

func startSMTPCapture(t *testing.T, mode string) (*smtpCapture, *x509.CertPool) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	capture := &smtpCapture{listener: listener, tlsConfig: &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}, mode: mode, messages: make(chan string, 10)}
	capture.wg.Add(1)
	go func() {
		defer capture.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			capture.mu.Lock()
			capture.connections = append(capture.connections, conn)
			capture.mu.Unlock()
			capture.wg.Add(1)
			go func() { defer capture.wg.Done(); capture.handle(conn) }()
		}
	}()
	t.Cleanup(func() {
		_ = capture.listener.Close()
		capture.mu.Lock()
		for _, conn := range capture.connections {
			_ = conn.Close()
		}
		capture.mu.Unlock()
		capture.wg.Wait()
	})
	return capture, roots
}
func (s *smtpCapture) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if s.mode == "tls" {
		wrapped := tls.Server(conn, s.tlsConfig)
		if wrapped.Handshake() != nil {
			return
		}
		conn = wrapped
	}
	reader := bufio.NewReader(conn)
	write := func(value string) bool { _, err := io.WriteString(conn, value); return err == nil }
	if s.mode == "stall" {
		_, _ = reader.ReadString('\n')
		return
	}
	if !write("220 local SMTP capture\r\n") {
		return
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(command, "EHLO"):
			if s.mode == "starttls" {
				if !write("250-local\r\n250 STARTTLS\r\n") {
					return
				}
			} else if !write("250 local\r\n") {
				return
			}
		case strings.HasPrefix(command, "HELO"), strings.HasPrefix(command, "MAIL FROM:"), strings.HasPrefix(command, "RCPT TO:"):
			if !write("250 OK\r\n") {
				return
			}
		case command == "STARTTLS":
			if !write("220 begin TLS\r\n") {
				return
			}
			wrapped := tls.Server(conn, s.tlsConfig)
			if wrapped.Handshake() != nil {
				return
			}
			conn = wrapped
			reader = bufio.NewReader(conn)
		case command == "DATA":
			if !write("354 send data\r\n") {
				return
			}
			var body strings.Builder
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if line == ".\r\n" {
					break
				}
				if strings.HasPrefix(line, "..") {
					line = line[1:]
				}
				body.WriteString(line)
			}
			if s.mode == "reject" {
				if !write("550 private SMTP rejection text\r\n") {
					return
				}
				continue
			}
			s.messages <- body.String()
			if !write("250 accepted\r\n") {
				return
			}
		case command == "QUIT":
			if s.mode != "quit-failure" {
				_ = write("221 bye\r\n")
			}
			return
		default:
			if !write("500 unsupported\r\n") {
				return
			}
		}
	}
}
func smtpCaptureConfig(c *smtpCapture, mode string, roots *x509.CertPool) SMTPConfig {
	return SMTPConfig{Host: "127.0.0.1", Port: c.listener.Addr().(*net.TCPAddr).Port, From: "zzfuse <noreply@example.test>", TLSMode: mode, Timeout: time.Second, RootCAs: roots}
}
func TestSMTPPrivateDelivery(t *testing.T) {
	for _, mode := range []string{"plain", "starttls", "tls", "quit-failure"} {
		t.Run(mode, func(t *testing.T) {
			capture, roots := startSMTPCapture(t, mode)
			transportMode := mode
			if mode == "quit-failure" {
				transportMode = "plain"
			}
			sender, err := NewSMTPEmailSender(smtpCaptureConfig(capture, transportMode, roots))
			if err != nil {
				t.Fatal(err)
			}
			body := "Private reset link: https://example.test/reset-password?token=private-test-token\nUnicode: café.\n"
			if err := sender.Send(context.Background(), "person@example.test", "Reset your password", body); err != nil {
				t.Fatal(err)
			}
			select {
			case raw := <-capture.messages:
				msg, err := mail.ReadMessage(strings.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
				if err != nil {
					t.Fatal(err)
				}
				if strings.ReplaceAll(string(decoded), "\r\n", "\n") != body || msg.Header.Get("To") != "<person@example.test>" || msg.Header.Get("Subject") != "Reset your password" {
					t.Fatal("SMTP did not privately deliver exact recipient, subject and body")
				}
			case <-time.After(time.Second):
				t.Fatal("no private SMTP delivery")
			}
		})
	}
}
func TestSMTPFailsClosed(t *testing.T) {
	cases := []struct {
		name, mode, transport string
		trust                 bool
	}{
		{"missing STARTTLS", "plain", "starttls", true}, {"untrusted TLS", "tls", "tls", false}, {"server rejected DATA", "reject", "plain", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture, roots := startSMTPCapture(t, tc.mode)
			if !tc.trust {
				roots = x509.NewCertPool()
			}
			sender, err := NewSMTPEmailSender(smtpCaptureConfig(capture, tc.transport, roots))
			if err != nil {
				t.Fatal(err)
			}
			if err := sender.Send(context.Background(), "person@example.test", "Reset", "private body"); !errors.Is(err, ErrEmailDelivery) {
				t.Fatal("SMTP failure was accepted or leaked server details", err)
			}
			select {
			case <-capture.messages:
				t.Fatal("unsafe mail was accepted")
			default:
			}
		})
	}
	capture, roots := startSMTPCapture(t, "plain")
	cfg := smtpCaptureConfig(capture, "plain", roots)
	cfg.Host = "example.com"
	if _, err := NewSMTPEmailSender(cfg); !errors.Is(err, ErrEmailConfiguration) {
		t.Fatal("external plaintext SMTP accepted")
	}
	cfg = smtpCaptureConfig(capture, "plain", roots)
	sender, _ := NewSMTPEmailSender(cfg)
	if err := sender.Send(context.Background(), "person@example.test\r\nBcc: attacker@example.test", "Reset", "body"); err == nil {
		t.Fatal("recipient header injection accepted")
	}
	if err := sender.Send(context.Background(), "person@example.test", "Reset\r\nBcc: attacker@example.test", "body"); err == nil {
		t.Fatal("subject header injection accepted")
	}
}
func TestSMTPCancellation(t *testing.T) {
	capture, roots := startSMTPCapture(t, "stall")
	sender, err := NewSMTPEmailSender(smtpCaptureConfig(capture, "plain", roots))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := sender.Send(ctx, "person@example.test", "Reset", "private body"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("context cancellation did not stop delivery")
	}
}
func TestSendEmailDisabled(t *testing.T) {
	t.Setenv("TRAZR_SMTP_HOST", "")
	if err := SendEmail(context.Background(), "person@example.test", "Subject", "Private token body"); !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatal("unconfigured delivery must fail closed")
	}
}
func TestResetDeliveryURLValidation(t *testing.T) {
	sender := emailSenderFunc(func(context.Context, string, string, string) error { return nil })
	for _, u := range []string{"/reset-password", "http://example.com/reset-password", "https://user:secret@example.com/reset-password", "https://example.com/reset-password#token"} {
		if _, err := NewPasswordResetDelivery(sender, u); err == nil {
			t.Fatal("unsafe frontend URL accepted")
		}
	}
	for _, u := range []string{"https://example.com/reset-password", "http://localhost:3000/reset-password", "http://127.0.0.1:3000/reset-password"} {
		if _, err := NewPasswordResetDelivery(sender, u); err != nil {
			t.Fatal(err)
		}
	}
}

type emailSenderFunc func(context.Context, string, string, string) error

func (f emailSenderFunc) Send(ctx context.Context, to, subject, body string) error {
	return f(ctx, to, subject, body)
}

func TestSMTPEnvironmentConfiguration(t *testing.T) {
	t.Setenv("TRAZR_SMTP_HOST", "localhost")
	t.Setenv("TRAZR_SMTP_PORT", "")
	t.Setenv("TRAZR_SMTP_FROM", "noreply@example.test")
	t.Setenv("TRAZR_SMTP_TLS_MODE", "")
	t.Setenv("TRAZR_SMTP_USERNAME", "")
	t.Setenv("TRAZR_SMTP_PASSWORD", "")
	t.Setenv("TRAZR_SMTP_TIMEOUT", "")
	t.Setenv("TRAZR_SMTP_CA_FILE", "")
	cfg, err := SMTPConfigFromEnv()
	if err != nil || cfg.Port != 587 || cfg.TLSMode != "starttls" || cfg.Timeout != 10*time.Second {
		t.Fatal("SMTP secure defaults incorrect", err)
	}
	cases := []struct{ name, value string }{{"TRAZR_SMTP_PORT", "0"}, {"TRAZR_SMTP_PORT", "65536"}, {"TRAZR_SMTP_PORT", "bad"}, {"TRAZR_SMTP_TLS_MODE", "unknown"}, {"TRAZR_SMTP_TIMEOUT", "-1s"}, {"TRAZR_SMTP_TIMEOUT", "2m"}, {"TRAZR_SMTP_FROM", "bad-address"}, {"TRAZR_SMTP_USERNAME", "unpaired-user"}, {"TRAZR_SMTP_CA_FILE", "/nonexistent/test-ca.pem"}}
	for _, tc := range cases {
		t.Run(tc.name+tc.value, func(t *testing.T) {
			t.Setenv(tc.name, tc.value)
			if _, err := SMTPConfigFromEnv(); !errors.Is(err, ErrEmailConfiguration) {
				t.Fatal("unsafe SMTP configuration accepted")
			}
		})
	}
}
