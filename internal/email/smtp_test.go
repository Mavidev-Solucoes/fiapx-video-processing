package email

import (
	"strings"
	"testing"
)

func TestEmailSenderConfiguration(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_PASSWORD", "")
	t.Setenv("SMTP_CA_FILE", "")
	sender, err := NewFromEnv()
	if err != nil || sender != nil {
		t.Fatalf("unconfigured sender should be nil, got %v %v", sender, err)
	}
	t.Setenv("SMTP_HOST", "mailpit")
	t.Setenv("SMTP_PORT", "1025")
	t.Setenv("SMTP_FROM", "no-reply@fiapx.test")
	sender, err = NewFromEnv()
	if err != nil || sender == nil {
		t.Fatalf("Mailpit TLS configuration: %v", err)
	}
	t.Setenv("SMTP_CA_FILE", "/does-not-exist/mailpit-ca.pem")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("missing CA file accepted")
	}
	t.Setenv("SMTP_CA_FILE", "")
	t.Setenv("SMTP_USERNAME", "login")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("incomplete SMTP credentials accepted")
	}
}

func TestFailureEmailContainsJobButNoSecrets(t *testing.T) {
	body := failureEmailMessage("no-reply@fiapx.test", "person@example.test",
		"ea8e33a2-7d9e-4e88-8829-9a74a79a8499", 41)
	for _, s := range []string{"To: person@example.test", "ID do trabalho: ea8e33a2",
		"Message-ID: <fiapx-failure-41", "Content-Type: text/plain"} {
		if !strings.Contains(body, s) {
			t.Fatalf("missing email content: %q", s)
		}
	}
	if strings.Contains(body, "password") {
		t.Fatal("email leaked credentials")
	}
	for _, invalid := range []string{"not-an-email", "a@example.test\r\nBcc: other@example.com", ""} {
		if validAddress(invalid) {
			t.Fatalf("unsafe recipient accepted: %q", invalid)
		}
	}
}
