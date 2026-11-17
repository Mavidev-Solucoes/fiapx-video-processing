package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/identity"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidToken = errors.New("invalid or expired access token")

const (
	tokenIssuer = "fiapx-video-processing"
	tokenTTL    = time.Hour
	tokenHeader = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9" // {"alg":"HS256","typ":"JWT"}
)

type claims struct {
	Subject  string `json:"sub"`
	Issuer   string `json:"iss"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

type Tokens struct {
	key []byte
	now func() time.Time
}

func NewTokens(secret string) (*Tokens, error) {
	if len([]byte(secret)) < 32 {
		return nil, errors.New("AUTH_JWT_SECRET must be at least 32 bytes")
	}
	return &Tokens{key: []byte(secret), now: time.Now}, nil
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 72 {
		return "", errors.New("password must contain between 12 and 72 bytes")
	}
	encoded, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(encoded), err
}

func CheckPassword(encoded, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password)) == nil
}

func (t *Tokens) Issue(subject string) (string, error) {
	if !identity.ValidUUID(subject) {
		return "", errors.New("subject must be a valid UUID")
	}
	now := t.now().UTC()
	content, err := json.Marshal(claims{
		Subject: subject,
		Issuer: tokenIssuer,
		IssuedAt: now.Unix(),
		Expires: now.Add(tokenTTL).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("encode claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(content)
	unsigned := tokenHeader + "." + payload
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(t.signature(unsigned)), nil
}

func (t *Tokens) Verify(raw string) (string, error) {
	if len(raw) > 4096 {
		return "", ErrInvalidToken
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] != tokenHeader {
		return "", ErrInvalidToken
	}
	unsigned := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, t.signature(unsigned)) {
		return "", ErrInvalidToken
	}
	content, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrInvalidToken
	}
	var token claims
	if err := json.Unmarshal(content, &token); err != nil {
		return "", ErrInvalidToken
	}
	now := t.now().UTC().Unix()
	if token.Issuer != tokenIssuer || !identity.ValidUUID(token.Subject) ||
		token.IssuedAt > now+60 || token.Expires <= now || token.Expires <= token.IssuedAt {
		return "", ErrInvalidToken
	}
	return token.Subject, nil
}

func (t *Tokens) signature(unsigned string) []byte {
	h := hmac.New(sha256.New, t.key)
	_, _ = h.Write([]byte(unsigned))
	return h.Sum(nil)
}
