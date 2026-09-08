package room_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/semmidev/wr/internal/room"
)

func newStore() room.Store {
	return room.NewMemStore()
}

var ctx = context.Background()

func TestActiveCount_EmptyRoom(t *testing.T) {
	s := newStore()
	count, err := s.ActiveCount(ctx, "room1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestAddActive_IsActive(t *testing.T) {
	s := newStore()
	exp := time.Now().Add(10 * time.Minute).UnixMilli()
	if err := s.AddActive(ctx, "r1", "user1", exp); err != nil {
		t.Fatal(err)
	}

	ok, err := s.IsActive(ctx, "r1", "user1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected user1 to be active")
	}
}

func TestIsActive_Expired(t *testing.T) {
	s := newStore()
	// Set expiry 1ms in the past.
	exp := time.Now().Add(-1 * time.Millisecond).UnixMilli()
	_ = s.AddActive(ctx, "r1", "expired-user", exp)

	ok, err := s.IsActive(ctx, "r1", "expired-user")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected expired-user to NOT be active")
	}
}

func TestActiveCount_LazyEviction(t *testing.T) {
	s := newStore()
	pastExp := time.Now().Add(-1 * time.Millisecond).UnixMilli()
	futureExp := time.Now().Add(10 * time.Minute).UnixMilli()

	_ = s.AddActive(ctx, "r1", "old-user", pastExp)
	_ = s.AddActive(ctx, "r1", "live-user", futureExp)

	count, err := s.ActiveCount(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 active (expired evicted), got %d", count)
	}
}

func TestExtendActive(t *testing.T) {
	s := newStore()
	exp := time.Now().Add(1 * time.Minute).UnixMilli()
	_ = s.AddActive(ctx, "r1", "u1", exp)

	newExp := time.Now().Add(10 * time.Minute).UnixMilli()
	_ = s.ExtendActive(ctx, "r1", "u1", newExp)

	ok, _ := s.IsActive(ctx, "r1", "u1")
	if !ok {
		t.Error("expected u1 to still be active after extend")
	}
}

func TestRemoveActive(t *testing.T) {
	s := newStore()
	exp := time.Now().Add(10 * time.Minute).UnixMilli()
	_ = s.AddActive(ctx, "r1", "u1", exp)
	_ = s.RemoveActive(ctx, "r1", "u1")

	ok, _ := s.IsActive(ctx, "r1", "u1")
	if ok {
		t.Error("expected u1 to be removed")
	}
}

func TestEnqueue_FIFO_Order(t *testing.T) {
	s := newStore()
	base := time.Now().UnixMilli()

	for i := 0; i < 5; i++ {
		item := room.QueueItem{
			ID:     fmt.Sprintf("u%d", i),
			RoomID: "r1",
			Score:  base + int64(i*100), // strictly increasing
		}
		pos, _, err := s.Enqueue(ctx, item)
		if err != nil {
			t.Fatalf("Enqueue u%d: %v", i, err)
		}
		if pos != int64(i) {
			t.Errorf("u%d: expected pos %d, got %d", i, i, pos)
		}
	}

	items, err := s.PopForAdmission(ctx, "r1", 3, "fifo")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	for i, it := range items {
		want := fmt.Sprintf("u%d", i)
		if it.ID != want {
			t.Errorf("pop[%d]: got %q, want %q", i, it.ID, want)
		}
	}
}

func TestEnqueue_Idempotent(t *testing.T) {
	s := newStore()
	item := room.QueueItem{ID: "u1", RoomID: "r1", Score: 1000}
	_, total1, _ := s.Enqueue(ctx, item)
	_, total2, _ := s.Enqueue(ctx, item) // same ID again

	if total1 != 1 || total2 != 1 {
		t.Errorf("expected total=1 on both enqueues, got %d and %d", total1, total2)
	}
}

func TestQueueLen_ExistsInQueue_RemoveFromQueue(t *testing.T) {
	s := newStore()
	item := room.QueueItem{ID: "u42", RoomID: "r1", Score: 999}
	_, _, _ = s.Enqueue(ctx, item)

	n, _ := s.QueueLen(ctx, "r1")
	if n != 1 {
		t.Errorf("expected len 1, got %d", n)
	}

	ok, _ := s.ExistsInQueue(ctx, "r1", "u42")
	if !ok {
		t.Error("expected u42 to exist in queue")
	}

	_ = s.RemoveFromQueue(ctx, "r1", "u42")

	ok, _ = s.ExistsInQueue(ctx, "r1", "u42")
	if ok {
		t.Error("expected u42 to be removed from queue")
	}
}

func TestPopForAdmission_Random(t *testing.T) {
	s := newStore()
	for i := 0; i < 10; i++ {
		_ = enqueueItem(s, "r1", fmt.Sprintf("u%d", i), int64(i*100))
	}

	items, err := s.PopForAdmission(ctx, "r1", 3, "random")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 random items, got %d", len(items))
	}

	remaining, _ := s.QueueLen(ctx, "r1")
	if remaining != 7 {
		t.Errorf("expected 7 remaining, got %d", remaining)
	}
}

func TestGetPosition(t *testing.T) {
	s := newStore()
	_ = enqueueItem(s, "r1", "a", 100)
	_ = enqueueItem(s, "r1", "b", 200)
	_ = enqueueItem(s, "r1", "c", 300)

	pos, total, found, err := s.GetPosition(ctx, "r1", "b")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected b to be found in queue")
	}
	if pos != 1 {
		t.Errorf("expected pos 1, got %d", pos)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}

	_, _, found, _ = s.GetPosition(ctx, "r1", "not-in-queue")
	if found {
		t.Error("expected not-in-queue to not be found")
	}
}

//nolint:unparam
func enqueueItem(s room.Store, roomID, id string, score int64) error {
	_, _, err := s.Enqueue(ctx, room.QueueItem{ID: id, RoomID: roomID, Score: score})
	return err
}
