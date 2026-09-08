package waitingroom

import (
	"context"
	"log/slog"
	"time"

	"github.com/semmidev/wr/internal/config"
	"github.com/semmidev/wr/internal/room"
)

// AdmissionWorker runs per-room background admission, releasing visitors from the queue
// at a rate matching new_users_per_minute. It uses an integer token-bucket accumulator
// to avoid floating-point rounding errors across ticks.
type AdmissionWorker struct {
	roomCfg      config.RoomConfig
	store        room.Store
	logger       *slog.Logger
	tickInterval time.Duration
}

// NewAdmissionWorker creates a worker for the given room.
// tickInterval controls how often the worker wakes up; 5s is a sensible default.
func NewAdmissionWorker(rc config.RoomConfig, store room.Store, logger *slog.Logger) *AdmissionWorker {
	return &AdmissionWorker{
		roomCfg:      rc,
		store:        store,
		logger:       logger,
		tickInterval: 5 * time.Second,
	}
}

// Start begins the admission loop. It blocks until ctx is cancelled.
func (w *AdmissionWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.tickInterval)
	defer ticker.Stop()

	// Token bucket accumulator (scaled by ticksPerMinute to stay in integer space).
	// Each tick we add newUsersPerMinute tokens; we consume them to admit visitors.
	// This avoids floating-point drift across many ticks.
	ticksPerMinute := int64(time.Minute / w.tickInterval)
	var bucket int64 // accumulated tokens * ticksPerMinute

	w.logger.Info("admission worker started",
		"room", w.roomCfg.ID,
		"new_per_min", w.roomCfg.NewUsersPerMinute,
		"method", w.roomCfg.QueueingMethod,
		"tick", w.tickInterval,
	)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("admission worker stopped", "room", w.roomCfg.ID)
			return

		case <-ticker.C:
			w.tick(ctx, ticksPerMinute, &bucket)
		}
	}
}

// tick is a single admission cycle.
func (w *AdmissionWorker) tick(ctx context.Context, ticksPerMinute int64, bucket *int64) {
	// Cleanup expired active sessions.
	now := time.Now().UnixMilli()
	if removed, err := w.store.CleanupExpiredActive(ctx, w.roomCfg.ID, now); err != nil {
		w.logger.Error("cleanup expired active failed", "room", w.roomCfg.ID, "err", err)
	} else if removed > 0 {
		w.logger.Info("cleaned expired sessions", "room", w.roomCfg.ID, "removed", removed)
	}

	// How many slots are open?
	active, err := w.store.ActiveCount(ctx, w.roomCfg.ID)
	if err != nil {
		w.logger.Error("active count failed", "room", w.roomCfg.ID, "err", err)
		return
	}
	available := w.roomCfg.TotalActiveUsers - active
	if available <= 0 {
		return
	}

	// Token bucket: add newUsersPerMinute tokens this tick (scaled by ticksPerMinute).
	*bucket += w.roomCfg.NewUsersPerMinute
	toAdmit := int(*bucket / ticksPerMinute)
	if toAdmit <= 0 {
		return
	}
	// Consume tokens.
	*bucket -= int64(toAdmit) * ticksPerMinute

	// Cap by available slots.
	if int64(toAdmit) > available {
		toAdmit = int(available)
	}

	items, err := w.store.PopForAdmission(ctx, w.roomCfg.ID, toAdmit, w.roomCfg.QueueingMethod)
	if err != nil {
		w.logger.Error("pop for admission failed", "room", w.roomCfg.ID, "err", err)
		return
	}
	if len(items) == 0 {
		return
	}

	exp := time.Now().Add(sessionDuration(w.roomCfg)).UnixMilli()
	for _, it := range items {
		if err := w.store.AddActive(ctx, w.roomCfg.ID, it.ID, exp); err != nil {
			w.logger.Error("add active failed", "room", w.roomCfg.ID, "id", it.ID, "err", err)
		}
	}
	w.logger.Info("admitted batch",
		"room", w.roomCfg.ID,
		"count", len(items),
		"active_after", active+int64(len(items)),
	)
}
