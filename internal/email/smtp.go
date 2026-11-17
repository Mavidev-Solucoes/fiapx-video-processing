package email

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/textproto"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Sender struct {
	host     string
	port     string
	from     string
	username string
	password string
	roots    *x509.CertPool
}

// NewFromEnv returns nil when SMTP is not configured. Implicit TLS is always
// required, including for the local Mailpit test relay.
func NewFromEnv() (*Sender, error) {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if host == "" {
		if os.Getenv("SMTP_USERNAME") != "" || os.Getenv("SMTP_PASSWORD") != "" {
			return nil, errors.New("SMTP_HOST is required when SMTP credentials are set")
		}
		return nil, nil
	}
	if strings.ContainsAny(host, ":/ \r\n") {
		return nil, errors.New("SMTP_HOST must be a hostname without scheme or port")
	}
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "465"
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, errors.New("SMTP_PORT must be between 1 and 65535")
	}
	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if !validAddress(from) {
		return nil, errors.New("SMTP_FROM must be a valid email address")
	}
	user, password := os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD")
	if (user == "") != (password == "") {
		return nil, errors.New("SMTP_USERNAME and SMTP_PASSWORD must be supplied together")
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if caFile := strings.TrimSpace(os.Getenv("SMTP_CA_FILE")); caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("cannot load SMTP CA certificate: %w", err)
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("SMTP_CA_FILE did not contain a valid certificate")
		}
	}
	return &Sender{host: host, port: port, from: from,
		username: user, password: password, roots: roots}, nil
}

// Keep the SMTP envelope strict: no quoted local parts, comments,
// internationalized addresses or characters that could inject ESMTP options.
var smtpAddressPattern = regexp.MustCompile("^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+[.][A-Za-z]{2,63}$")

func validAddress(address string) bool {
	if address == "" || len(address) > 254 || !smtpAddressPattern.MatchString(address) {
		return false
	}
	value, err := mail.ParseAddress(address)
	return err == nil && value.Address == address
}

// SendFailure operates exclusively inside a TLS 1.2+ connection whose
// hostname and certificate have been verified. SMTP relay acceptance is not
// proof that an end recipient's mailbox received the message.
func (s *Sender) SendFailure(ctx context.Context, recipient, jobID string, notificationID int64) error {
	if !validAddress(recipient) {
		return errors.New("invalid email recipient")
	}
	address := net.JoinHostPort(s.host, s.port)
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 5 * time.Second},
		Config: &tls.Config{
			ServerName: s.host,
			MinVersion: tls.VersionTLS12,
			RootCAs: s.roots,
		},
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("encrypted SMTP connection failure: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	cancelWatchDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-cancelWatchDone:
		}
	}()
	defer close(cancelWatchDone)

	// Issue a minimal SMTP exchange on the already-encrypted connection.
	// No plain TCP fallback or STARTTLS downgrade path exists.
	smtpConn := textproto.NewConn(conn)
	defer smtpConn.Close()
	if _, _, err := smtpConn.ReadResponse(220); err != nil {
		return fmt.Errorf("SMTP greeting rejected: %w", err)
	}
	if err := smtpCommand(smtpConn, 250, "EHLO fiapx.local"); err != nil {
		return err
	}
	if s.username != "" {
		token := base64.StdEncoding.EncodeToString(
			[]byte("\x00" + s.username + "\x00" + s.password),
		)
		if err := smtpCommand(smtpConn, 235, "AUTH PLAIN %s", token); err != nil {
			return fmt.Errorf("encrypted SMTP authentication failed: %w", err)
		}
	}
	if err := smtpCommand(smtpConn, 250, "MAIL FROM:<%s>", s.from); err != nil {
		return err
	}
	if err := smtpCommand(smtpConn, 250, "RCPT TO:<%s>", recipient); err != nil {
		return err
	}
	if err := smtpCommand(smtpConn, 354, "DATA"); err != nil {
		return err
	}
	writer := smtpConn.DotWriter()
	if _, err := io.WriteString(writer, failureEmailMessage(s.from, recipient, jobID, notificationID)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("SMTP message write failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("SMTP message termination failed: %w", err)
	}
	if _, _, err := smtpConn.ReadResponse(250); err != nil {
		return fmt.Errorf("SMTP relay did not accept message: %w", err)
	}
	_ = smtpCommand(smtpConn, 221, "QUIT")
	return nil
}

func smtpCommand(conn *textproto.Conn, expected int, format string, args ...any) error {
	if err := conn.PrintfLine(format, args...); err != nil {
		return fmt.Errorf("SMTP command write failed: %w", err)
	}
	if _, _, err := conn.ReadResponse(expected); err != nil {
		return fmt.Errorf("SMTP command rejected: %w", err)
	}
	return nil
}

func failureEmailMessage(from, recipient, jobID string, notificationID int64) string {
	body := "Olá,\r\n\r\nNão foi possível processar um vídeo enviado ao FIAP X.\r\n" +
		"ID do trabalho: " + jobID + "\r\n\r\n" +
		"Acesse o FIAP X para consultar os detalhes e enviar outro vídeo, se necessário.\r\n\r\n" +
		"Esta é uma mensagem automática.\r\n"
	return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: FIAP X - falha no processamento de video\r\n"+
		"Date: %s\r\nMessage-ID: <fiapx-failure-%d@fiapx.local>\r\n"+
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n"+
		"Content-Transfer-Encoding: 8bit\r\n\r\n%s",
		from, recipient, time.Now().UTC().Format(time.RFC1123Z), notificationID, body)
}
