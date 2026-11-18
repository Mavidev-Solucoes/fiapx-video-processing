package email

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"testing"
)

func TestEmailSenderRejectsUntrustedEndpointAndRecipient(t *testing.T) {
	sender := &Sender{
		host: "127.0.0.1", port: "1", from: "notifications@example.test",
		roots: x509.NewCertPool(),
	}
	if err := sender.SendFailure(context.Background(),
		"evil@example.test\r\nBcc: victim@example.test", "job", 3); err == nil {
		t.Fatal("CRLF recipient injection allowed")
	}
	if err := sender.SendFailure(context.Background(), "person@example.test",
		"f8162870-e1f5-4809-8c76-807ad16d42e1", 3); err == nil {
		t.Fatal("unreachable TLS relay accepted message")
	}
}

func TestEmailSenderRejectsPlaintextServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _ = connection.Write([]byte("220 fake plaintext SMTP server\r\n"))
	}()
	address := listener.Addr().(*net.TCPAddr)
	sender := &Sender{
		host: "127.0.0.1", port: fmt.Sprint(address.Port),
		from: "notifications@example.test", roots: x509.NewCertPool(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sender.SendFailure(ctx, "person@example.test", "job-identifier", 9); err == nil {
		t.Fatal("plaintext SMTP greeting accepted without verified TLS")
	}
}
