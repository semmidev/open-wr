// Package cookie provides HMAC-signed session cookie creation and verification.
//
// Cookie format (base64url-encoded outer layer):
//
//	id|roomID|status|iat|exp|hmac-sig
//
// where iat and exp are Unix timestamps in seconds.
// status is one of: "queued" | "active"
package cookie

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidToken is returned when a token cannot be verified.
var ErrInvalidToken = errors.New("cookie: invalid token")

// ErrExpiredToken is returned when a token has passed its expiry.
var ErrExpiredToken = errors.New("cookie: token expired")

// ErrWeakSecret is returned when the signing secret is too short.
var ErrWeakSecret = errors.New("cookie: secret must be at least 32 bytes")

const minSecretLen = 32

// Status values for cookie payload.
const (
	StatusQueued = "queued"
	StatusActive = "active"
)

// Payload represents the decoded, verified contents of a waiting-room cookie.
// All time fields are Unix seconds.
type Payload struct {
	ID       string
	RoomID   string
	Status   string // StatusQueued | StatusActive
	IssuedAt int64  // Unix seconds
	ExpAt    int64  // Unix seconds
}

// Signer creates and verifies HMAC-SHA256 signed cookies.
type Signer struct {
	secret []byte
}

// NewSigner creates a new Signer. Returns an error if the secret is shorter than 32 bytes.
func NewSigner(secret string) (*Signer, error) {
	if len(secret) < minSecretLen {
		return nil, ErrWeakSecret
	}
	return &Signer{secret: []byte(secret)}, nil
}

// MustNewSigner is like NewSigner but panics on error. Use only in tests or init paths
// where a weak secret is a programming error.
func MustNewSigner(secret string) *Signer {
	s, err := NewSigner(secret)
	if err != nil {
		panic(err)
	}
	return s
}

// Sign encodes and signs a Payload, returning the cookie value string.
// All time values in Payload must be Unix seconds.
func (s *Signer) Sign(p Payload) (string, error) {
	msg := fmt.Sprintf("%s|%s|%s|%d|%d", p.ID, p.RoomID, p.Status, p.IssuedAt, p.ExpAt)
	sig := s.sign(msg)
	cookieVal := msg + "|" + sig
	// Outer base64url encoding avoids cookie special-char issues.
	return base64.RawURLEncoding.EncodeToString([]byte(cookieVal)), nil
}

// Verify decodes and validates a cookie value. Returns the Payload and nil on success.
// Returns ErrInvalidToken if the HMAC is wrong or the format is malformed.
// Returns ErrExpiredToken if the token has passed its expiry time.
func (s *Signer) Verify(token string) (Payload, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Payload{}, ErrInvalidToken
	}

	parts := strings.Split(string(decoded), "|")
	if len(parts) != 6 {
		return Payload{}, ErrInvalidToken
	}

	id, roomID, status, iatStr, expStr, sig := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5]
	msg := strings.Join(parts[:5], "|")

	if !hmac.Equal([]byte(s.sign(msg)), []byte(sig)) {
		return Payload{}, ErrInvalidToken
	}

	iat, err := strconv.ParseInt(iatStr, 10, 64)
	if err != nil {
		return Payload{}, ErrInvalidToken
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return Payload{}, ErrInvalidToken
	}

	if time.Now().Unix() > exp {
		return Payload{}, ErrExpiredToken
	}

	return Payload{
		ID:       id,
		RoomID:   roomID,
		Status:   status,
		IssuedAt: iat,
		ExpAt:    exp,
	}, nil
}

// CookieName returns the namespaced cookie name for a given room.
// Format: {base}_{roomID}. Falls back to "__owr" if base is empty.
func (s *Signer) CookieName(base, roomID string) string {
	if base == "" {
		base = "__owr"
	}
	return fmt.Sprintf("%s_%s", base, roomID)
}

// sign computes the base64url-encoded HMAC-SHA256 of msg.
func (s *Signer) sign(msg string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
