package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type Sender struct {
	host            string
	port            string
	from            string
	username        string
	password        string
	allowPlainLocal bool
}

// NewFromEnv returns nil when SMTP_HOST is absent; in-app notifications still
// work and pending emails remain durable until a sender is configured.
// SMTP_ALLOW_PLAINTEXT is reserved for the Docker-only Mailpit demonstration.
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
		port = "587"
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
	allowPlain := os.Getenv("SMTP_ALLOW_PLAINTEXT") == "true"
	if allowPlain && host != "mailpit" && host != "localhost" && host != "127.0.0.1" {
		return nil, errors.New("plaintext SMTP is only supported for local Mailpit")
	}
	if allowPlain && user != "" {
		return nil, errors.New("SMTP authentication requires encrypted transport")
	}
	return &Sender{host: host, port: port, from: from, username: user,
		password: password, allowPlainLocal: allowPlain}, nil
}

func validAddress(address string) bool {
	if address == "" || len(address) > 254 || strings.ContainsAny(address, "\r\n") {
		return false
	}
	value, err := mail.ParseAddress(address)
	return err == nil && value.Address == address && strings.Contains(address, "@")
}

// SendFailure submits the message over SMTP. The database dispatcher handles
// retries; a successful SMTP DATA response counts as delivered to the relay,
// not as proof that a remote mailbox actually received the email.
func (s *Sender) SendFailure(ctx context.Context, recipient, jobID string, notificationID int64) error {
	if !validAddress(recipient) {
		return errors.New("invalid email recipient")
	}
	address := net.JoinHostPort(s.host, s.port)
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("SMTP connection failure: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	stopCancellation := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stopCancellation:
		}
	}()
	defer close(stopCancellation)

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("SMTP greeting failure: %w", err)
	}
	defer client.Close()

	if supported, _ := client.Extension("STARTTLS"); supported {
		err = client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return fmt.Errorf("SMTP TLS negotiation failure: %w", err)
		}
	} else {
		return errors.New("SMTP server does not support required STARTTLS")
	}
	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("SMTP sender rejected: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("SMTP recipient rejected: %w", err)
	}
	data, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP data command failed: %w", err)
	}
	if _, err := io.WriteString(data, failureEmailMessage(s.from, recipient, jobID, notificationID)); err != nil {
		_ = data.Close()
		return fmt.Errorf("SMTP message write failed: %w", err)
	}
	if err := data.Close(); err != nil {
		return fmt.Errorf("SMTP message was not accepted: %w", err)
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
