package waitingroom

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/semmidev/wr/internal/config"
	"github.com/semmidev/wr/internal/cookie"
	"github.com/semmidev/wr/internal/room"
)

// Action constants for Decision.
const (
	ActionAllow  = "allow"
	ActionQueue  = "queue"
	ActionBypass = "bypass"
)

// Decision is the result of CheckRequest for a single incoming HTTP request.
type Decision struct {
	Action        string // ActionAllow | ActionQueue | ActionBypass
	Room          config.RoomConfig
	CookiePayload *cookie.Payload
	Position      int64 // zero-based queue position (only when Action == ActionQueue)
	TotalQueued   int64
	EstimatedWait int64 // seconds
	IsNewQueue    bool  // true if this is a newly issued cookie (either queue or direct admit)
}

// Service implements the core waiting room logic.
// It is safe for concurrent use.
type Service struct {
	mu     sync.RWMutex
	rooms  map[string]config.RoomConfig
	store  room.Store
	signer *cookie.Signer
	logger *slog.Logger
}

// NewService constructs a Service from the provided config.
func NewService(cfg *config.Config, store room.Store, signer *cookie.Signer, logger *slog.Logger) *Service {
	m := make(map[string]config.RoomConfig, len(cfg.Rooms))
	for _, r := range cfg.Rooms {
		m[r.ID] = r
	}
	return &Service{
		rooms:  m,
		store:  store,
		signer: signer,
		logger: logger,
	}
}

// GetRoom returns the config for roomID. ok is false if the room is unknown.
func (s *Service) GetRoom(roomID string) (config.RoomConfig, bool) {
	s.mu.RLock()
	r, ok := s.rooms[roomID]
	s.mu.RUnlock()
	return r, ok
}

// ListRooms returns all registered room configs.
func (s *Service) ListRooms() []config.RoomConfig {
	s.mu.RLock()
	list := make([]config.RoomConfig, 0, len(s.rooms))
	for _, r := range s.rooms {
		list = append(list, r)
	}
	s.mu.RUnlock()
	return list
}

// CheckRequest implements the core waiting room decision logic:
//  1. Valid active cookie → allow + optionally extend session
//  2. Valid queued cookie that was promoted → allow (upgrade cookie)
//  3. Valid queued cookie still in queue → queue (return position)
//  4. No valid cookie + capacity available → admit directly
//  5. No valid cookie + at capacity → enqueue
//
// cookieVal is the raw signed cookie value (empty string if absent).
// ip and ua are used for queue metadata.
func (s *Service) CheckRequest(ctx context.Context, roomID, cookieVal, ip, ua string) (Decision, error) {
	rcfg, ok := s.GetRoom(roomID)
	if !ok {
		return Decision{Action: ActionBypass}, nil
	}
	if !rcfg.Enabled {
		return Decision{Action: ActionBypass, Room: rcfg}, nil
	}

	// 1 & 2 & 3 — evaluate existing cookie.
	if cookieVal != "" {
		if dec, handled, err := s.evaluateCookie(ctx, rcfg, cookieVal); handled {
			return dec, err
		}
	}

	// 4 & 5 — no valid existing cookie: decide admit vs queue.
	return s.admitOrQueue(ctx, rcfg, ip, ua)
}

// evaluateCookie attempts to verify and act on an existing cookie value.
// handled=true means a final Decision was produced (caller should return it).
// handled=false means the cookie was invalid/expired and the caller should fall through.
func (s *Service) evaluateCookie(ctx context.Context, rcfg config.RoomConfig, cookieVal string) (Decision, bool, error) {
	payload, err := s.signer.Verify(cookieVal)
	if err != nil {
		// Expired or invalid — fall through to re-evaluation.
		return Decision{}, false, nil //nolint:nilerr // Intentional: invalid/expired cookie falls through to re-evaluation
	}
	if payload.RoomID != rcfg.ID {
		return Decision{}, false, nil
	}

	switch payload.Status {
	case cookie.StatusActive:
		isActive, err := s.store.IsActive(ctx, rcfg.ID, payload.ID)
		if err != nil {
			return Decision{}, true, err
		}
		if isActive {
			if !rcfg.DisableSessionRenewal {
				newExp := time.Now().Add(sessionDuration(rcfg)).UnixMilli()
				_ = s.store.ExtendActive(ctx, rcfg.ID, payload.ID, newExp) // best-effort; log on error
			}
			return Decision{Action: ActionAllow, Room: rcfg, CookiePayload: &payload}, true, nil
		}
		// Session expired in store but cookie not yet expired — fall through to re-evaluate.

	case cookie.StatusQueued:
		// Check if the admission worker has already promoted this visitor.
		isActive, err := s.store.IsActive(ctx, rcfg.ID, payload.ID)
		if err != nil {
			return Decision{}, true, err
		}
		if isActive {
			upgraded := payload
			upgraded.Status = cookie.StatusActive
			s.logger.Info("visitor promoted from queue", "room", rcfg.ID, "id", payload.ID)
			return Decision{Action: ActionAllow, Room: rcfg, CookiePayload: &upgraded}, true, nil
		}

		pos, total, found, err := s.store.GetPosition(ctx, rcfg.ID, payload.ID)
		if err != nil {
			return Decision{}, true, err
		}
		if found {
			eta := estimateWait(pos, rcfg.NewUsersPerMinute)
			return Decision{
				Action:        ActionQueue,
				Room:          rcfg,
				CookiePayload: &payload,
				Position:      pos,
				TotalQueued:   total,
				EstimatedWait: eta,
			}, true, nil
		}
		// Queued cookie but not found in store — re-queue below.
	}

	return Decision{}, false, nil
}

// admitOrQueue decides whether to admit directly (no queue) or enqueue the visitor.
func (s *Service) admitOrQueue(ctx context.Context, rcfg config.RoomConfig, ip, ua string) (Decision, error) {
	activeCount, err := s.store.ActiveCount(ctx, rcfg.ID)
	if err != nil {
		return Decision{}, err
	}

	shouldQueue := rcfg.QueueAll || activeCount >= rcfg.TotalActiveUsers

	if !shouldQueue {
		return s.admitDirect(ctx, rcfg, activeCount)
	}
	return s.enqueue(ctx, rcfg, ip, ua)
}

// admitDirect creates a new active session without going through the queue.
func (s *Service) admitDirect(ctx context.Context, rcfg config.RoomConfig, currentActive int64) (Decision, error) {
	id := uuid.NewString()
	exp := time.Now().Add(sessionDuration(rcfg))

	if err := s.store.AddActive(ctx, rcfg.ID, id, exp.UnixMilli()); err != nil {
		return Decision{}, err
	}

	payload := &cookie.Payload{
		ID:       id,
		RoomID:   rcfg.ID,
		Status:   cookie.StatusActive,
		IssuedAt: time.Now().Unix(),
		ExpAt:    exp.Unix(),
	}
	s.logger.Info("admit direct", "room", rcfg.ID, "id", id, "active", currentActive+1)
	return Decision{
		Action:        ActionAllow,
		Room:          rcfg,
		CookiePayload: payload,
		IsNewQueue:    true,
	}, nil
}

// enqueue places a new visitor in the waiting queue.
func (s *Service) enqueue(ctx context.Context, rcfg config.RoomConfig, ip, ua string) (Decision, error) {
	id := uuid.NewString()
	now := time.Now()

	item := room.QueueItem{
		ID:         id,
		RoomID:     rcfg.ID,
		Score:      now.UnixMilli(),
		EnqueuedAt: now,
		IP:         ip,
		UserAgent:  ua,
	}
	pos, total, err := s.store.Enqueue(ctx, item)
	if err != nil {
		return Decision{}, err
	}

	payload := &cookie.Payload{
		ID:       id,
		RoomID:   rcfg.ID,
		Status:   cookie.StatusQueued,
		IssuedAt: now.Unix(),
		ExpAt:    now.Add(30 * time.Minute).Unix(), // queue cookie TTL
	}
	eta := estimateWait(pos, rcfg.NewUsersPerMinute)
	s.logger.Info("enqueue", "room", rcfg.ID, "id", id, "pos", pos, "total", total)

	return Decision{
		Action:        ActionQueue,
		Room:          rcfg,
		CookiePayload: payload,
		Position:      pos,
		TotalQueued:   total,
		EstimatedWait: eta,
		IsNewQueue:    true,
	}, nil
}

// GetStatus returns the current status of a visitor identified by id.
func (s *Service) GetStatus(ctx context.Context, roomID, id string) (pos, total, activeCount, eta int64, isActive bool, err error) {
	activeCount, err = s.store.ActiveCount(ctx, roomID)
	if err != nil {
		return
	}

	var active bool
	active, err = s.store.IsActive(ctx, roomID, id)
	if err != nil {
		return
	}
	if active {
		isActive = true
		return
	}

	var found bool
	pos, total, found, err = s.store.GetPosition(ctx, roomID, id)
	if err != nil || !found {
		return
	}

	rcfg, ok := s.GetRoom(roomID)
	if ok {
		eta = estimateWait(pos, rcfg.NewUsersPerMinute)
	}
	return
}

// Stats returns a snapshot of the room's admission state.
func (s *Service) Stats(ctx context.Context, roomID string) (room.Stats, error) {
	rcfg, ok := s.GetRoom(roomID)
	if !ok {
		return room.Stats{}, errors.New("room not found: " + roomID)
	}

	active, err := s.store.ActiveCount(ctx, roomID)
	if err != nil {
		return room.Stats{}, err
	}
	queued, err := s.store.QueueLen(ctx, roomID)
	if err != nil {
		return room.Stats{}, err
	}

	return room.Stats{
		RoomID:           roomID,
		ActiveCount:      active,
		QueuedCount:      queued,
		NewUsersPerMin:   rcfg.NewUsersPerMinute,
		TotalActiveLimit: rcfg.TotalActiveUsers,
		QueueingMethod:   rcfg.QueueingMethod,
		IsQueueing:       rcfg.QueueAll || active >= rcfg.TotalActiveUsers,
	}, nil
}

// sessionDuration converts room config minutes into a time.Duration.
func sessionDuration(rcfg config.RoomConfig) time.Duration {
	return time.Duration(rcfg.SessionDurationMinutes) * time.Minute
}

// estimateWait calculates estimated wait in seconds given zero-based position and admission rate.
func estimateWait(position, newPerMin int64) int64 {
	if newPerMin <= 0 {
		newPerMin = 60
	}
	perSec := float64(newPerMin) / 60.0
	if perSec <= 0 {
		perSec = 1
	}
	return int64(math.Ceil(float64(position) / perSec))
}
