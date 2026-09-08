package cookie_test

import (
	"errors"
	"testing"
	"time"

	"github.com/semmidev/wr/internal/cookie"
)

const testSecret = "a-very-secure-secret-for-testing-1234"

func TestNewSigner_WeakSecret(t *testing.T) {
	_, err := cookie.NewSigner("short")
	if !errors.Is(err, cookie.ErrWeakSecret) {
		t.Fatalf("expected ErrWeakSecret, got %v", err)
	}
}

func TestSignVerify_RoundTrip(t *testing.T) {
	s := cookie.MustNewSigner(testSecret)

	now := time.Now().Unix()
	payload := cookie.Payload{
		ID:       "abc-123",
		RoomID:   "room_1",
		Status:   cookie.StatusActive,
		IssuedAt: now,
		ExpAt:    now + 600, // 10 min
	}

	token, err := s.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if token == "" {
		t.Fatal("Sign returned empty token")
	}

	got, err := s.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if got.ID != payload.ID {
		t.Errorf("ID: got %q, want %q", got.ID, payload.ID)
	}
	if got.RoomID != payload.RoomID {
		t.Errorf("RoomID: got %q, want %q", got.RoomID, payload.RoomID)
	}
	if got.Status != payload.Status {
		t.Errorf("Status: got %q, want %q", got.Status, payload.Status)
	}
}

func TestVerify_ExpiredToken(t *testing.T) {
	s := cookie.MustNewSigner(testSecret)

	past := time.Now().Unix() - 3600 // 1 hour ago
	payload := cookie.Payload{
		ID:       "expired-id",
		RoomID:   "room_1",
		Status:   cookie.StatusQueued,
		IssuedAt: past - 600,
		ExpAt:    past, // already expired
	}

	token, err := s.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = s.Verify(token)
	if !errors.Is(err, cookie.ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestVerify_InvalidToken(t *testing.T) {
	s := cookie.MustNewSigner(testSecret)

	cases := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"garbage", "not-a-valid-base64-token!!!"},
		{"wrong-separator", "abc.def.ghi"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Verify(tc.token)
			if !errors.Is(err, cookie.ErrInvalidToken) {
				t.Fatalf("expected ErrInvalidToken, got %v", err)
			}
		})
	}
}

func TestVerify_TamperedHMAC(t *testing.T) {
	s := cookie.MustNewSigner(testSecret)

	now := time.Now().Unix()
	token, _ := s.Sign(cookie.Payload{
		ID: "x", RoomID: "r", Status: cookie.StatusActive,
		IssuedAt: now, ExpAt: now + 600,
	})

	// Flip a character in the token to tamper with it.
	b := []byte(token)
	b[len(b)-2] ^= 0xFF
	tampered := string(b)

	_, err := s.Verify(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token, got nil")
	}
}

func TestCookieName(t *testing.T) {
	s := cookie.MustNewSigner(testSecret)

	cases := []struct {
		base, roomID, want string
	}{
		{"__owr", "flash_sale", "__owr_flash_sale"},
		{"", "room1", "__owr_room1"},
		{"custom", "x", "custom_x"},
	}
	for _, tc := range cases {
		got := s.CookieName(tc.base, tc.roomID)
		if got != tc.want {
			t.Errorf("CookieName(%q,%q) = %q, want %q", tc.base, tc.roomID, got, tc.want)
		}
	}
}
