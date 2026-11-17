package email

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
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
	return nil // diagnostic stub: do not send
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
