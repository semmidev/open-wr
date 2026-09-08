package waitingroom_test

import (
	"context"
	"testing"
	"time"

	"github.com/semmidev/wr/internal/config"
	"github.com/semmidev/wr/internal/cookie"
	"github.com/semmidev/wr/internal/room"
	"github.com/semmidev/wr/internal/waitingroom"
	"log/slog"
	"os"
)

const (
	testSecret = "a-secure-test-secret-for-hmac-abc!"
	roomID     = "test_room"
)

func testRoom() config.RoomConfig {
	return config.RoomConfig{
		ID:                     roomID,
		Name:                   "Test Room",
		Enabled:                true,
		QueueAll:               false,
		NewUsersPerMinute:      60,
		TotalActiveUsers:       2,
		SessionDurationMinutes: 10,
		QueueingMethod:         "fifo",
		CookieName:             "__owr",
	}
}

func testService() (*waitingroom.Service, room.Store, *cookie.Signer) {
	cfg := &config.Config{
		Rooms: []config.RoomConfig{testRoom()},
	}
	store := room.NewMemStore()
	signer := cookie.MustNewSigner(testSecret)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := waitingroom.NewService(cfg, store, signer, logger)
	return svc, store, signer
}

var ctx = context.Background()

func TestCheckRequest_DirectAdmit(t *testing.T) {
	svc, _, _ := testService()

	dec, err := svc.CheckRequest(ctx, roomID, "", "1.2.3.4", "go-test")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != waitingroom.ActionAllow {
		t.Errorf("expected ActionAllow, got %q", dec.Action)
	}
	if dec.CookiePayload == nil {
		t.Fatal("expected CookiePayload to be set")
	}
	if dec.CookiePayload.Status != cookie.StatusActive {
		t.Errorf("expected cookie status active, got %q", dec.CookiePayload.Status)
	}
}

func TestCheckRequest_QueueWhenFull(t *testing.T) {
	svc, store, _ := testService()

	// Fill capacity.
	exp := time.Now().Add(10 * time.Minute).UnixMilli()
	_ = store.AddActive(ctx, roomID, "existing-1", exp)
	_ = store.AddActive(ctx, roomID, "existing-2", exp)

	dec, err := svc.CheckRequest(ctx, roomID, "", "1.2.3.4", "go-test")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != waitingroom.ActionQueue {
		t.Errorf("expected ActionQueue, got %q", dec.Action)
	}
	if dec.CookiePayload == nil || dec.CookiePayload.Status != cookie.StatusQueued {
		t.Error("expected queued cookie payload")
	}
	if !dec.IsNewQueue {
		t.Error("expected IsNewQueue=true for new enqueue")
	}
}

func TestCheckRequest_ActiveCookieAllow(t *testing.T) {
	svc, store, signer := testService()

	// Create active session.
	id := "test-visitor"
	exp := time.Now().Add(10 * time.Minute).UnixMilli()
	_ = store.AddActive(ctx, roomID, id, exp)

	payload := cookie.Payload{
		ID:       id,
		RoomID:   roomID,
		Status:   cookie.StatusActive,
		IssuedAt: time.Now().Unix(),
		ExpAt:    time.Now().Add(10 * time.Minute).Unix(),
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := svc.CheckRequest(ctx, roomID, signed, "1.2.3.4", "go-test")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != waitingroom.ActionAllow {
		t.Errorf("expected ActionAllow for valid active cookie, got %q", dec.Action)
	}
}

func TestCheckRequest_QueuedCookieStillQueued(t *testing.T) {
	svc, store, signer := testService()

	// Fill capacity so visitor gets queued.
	exp := time.Now().Add(10 * time.Minute).UnixMilli()
	_ = store.AddActive(ctx, roomID, "slot-1", exp)
	_ = store.AddActive(ctx, roomID, "slot-2", exp)

	// First request: visitor gets queued.
	dec, err := svc.CheckRequest(ctx, roomID, "", "1.2.3.4", "go-test")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != waitingroom.ActionQueue {
		t.Fatalf("expected queue, got %q", dec.Action)
	}

	// Sign the queued cookie and reuse it.
	signed, _ := signer.Sign(*dec.CookiePayload)

	// Second request with existing queued cookie.
	dec2, err := svc.CheckRequest(ctx, roomID, signed, "1.2.3.4", "go-test")
	if err != nil {
		t.Fatal(err)
	}
	if dec2.Action != waitingroom.ActionQueue {
		t.Errorf("expected queue on second request, got %q", dec2.Action)
	}
	// Should return same position.
	_ = store
}

func TestCheckRequest_PromotedByWorker(t *testing.T) {
	svc, store, signer := testService()

	// Fill capacity so visitor gets queued.
	exp := time.Now().Add(10 * time.Minute).UnixMilli()
	_ = store.AddActive(ctx, roomID, "slot-1", exp)
	_ = store.AddActive(ctx, roomID, "slot-2", exp)

	dec, _ := svc.CheckRequest(ctx, roomID, "", "1.2.3.4", "go-test")
	visitorID := dec.CookiePayload.ID
	signed, _ := signer.Sign(*dec.CookiePayload)

	// Simulate worker promoting the visitor.
	newExp := time.Now().Add(10 * time.Minute).UnixMilli()
	_ = store.AddActive(ctx, roomID, visitorID, newExp)

	// Next request: visitor should be allowed.
	dec2, err := svc.CheckRequest(ctx, roomID, signed, "1.2.3.4", "go-test")
	if err != nil {
		t.Fatal(err)
	}
	if dec2.Action != waitingroom.ActionAllow {
		t.Errorf("expected ActionAllow after promotion, got %q", dec2.Action)
	}
	if dec2.CookiePayload == nil || dec2.CookiePayload.Status != cookie.StatusActive {
		t.Error("expected upgraded active cookie")
	}
}

func TestCheckRequest_UnknownRoom_Bypass(t *testing.T) {
	svc, _, _ := testService()

	dec, err := svc.CheckRequest(ctx, "nonexistent_room", "", "1.2.3.4", "")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != waitingroom.ActionBypass {
		t.Errorf("expected ActionBypass for unknown room, got %q", dec.Action)
	}
}

func TestGetStatus_ActiveVisitor(t *testing.T) {
	svc, store, _ := testService()

	id := "visitor-x"
	exp := time.Now().Add(5 * time.Minute).UnixMilli()
	_ = store.AddActive(ctx, roomID, id, exp)

	_, _, _, _, isActive, err := svc.GetStatus(ctx, roomID, id)
	if err != nil {
		t.Fatal(err)
	}
	if !isActive {
		t.Error("expected isActive=true")
	}
}
