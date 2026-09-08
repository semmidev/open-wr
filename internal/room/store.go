// Package room defines the persistent Store interface used by the waiting room.
//
// All expiry values exchanged with the Store are Unix milliseconds (int64).
// This is consistent with Redis ZSET score semantics and time.UnixMilli().
package room

import "context"

// Store is the persistence layer for the waiting room.
// All expiry parameters are Unix milliseconds.
type Store interface {
	// --- Active session set ---

	// ActiveCount returns the number of currently-valid active sessions.
	// Implementations should evict expired sessions before counting.
	ActiveCount(ctx context.Context, roomID string) (int64, error)

	// IsActive reports whether the given ID holds a valid (non-expired) active session.
	IsActive(ctx context.Context, roomID, id string) (bool, error)

	// AddActive registers id as active with the given expiry (Unix millis).
	AddActive(ctx context.Context, roomID, id string, expiresAtMillis int64) error

	// ExtendActive updates the expiry of an existing active session (Unix millis).
	// If the ID does not exist the call is a no-op.
	ExtendActive(ctx context.Context, roomID, id string, newExpiryMillis int64) error

	// RemoveActive forcefully removes an active session.
	RemoveActive(ctx context.Context, roomID, id string) error

	// CleanupExpiredActive deletes sessions whose expiry ≤ nowMillis.
	// Returns the number of entries removed.
	CleanupExpiredActive(ctx context.Context, roomID string, nowMillis int64) (int64, error)

	// --- Queue ---

	// Enqueue adds item to the queue (idempotent by ID).
	// Returns the zero-based position and total queue length after insertion.
	Enqueue(ctx context.Context, item QueueItem) (position int64, total int64, err error)

	// GetPosition returns the zero-based position of id in the queue and the total length.
	// found is false if the ID is not in the queue.
	GetPosition(ctx context.Context, roomID, id string) (pos int64, total int64, found bool, err error)

	// PopForAdmission atomically removes up to n items from the queue.
	// method must be "fifo" or "random".
	PopForAdmission(ctx context.Context, roomID string, n int, method string) ([]QueueItem, error)

	// RemoveFromQueue removes a single item from the queue by ID.
	RemoveFromQueue(ctx context.Context, roomID, id string) error

	// QueueLen returns the current number of entries in the queue.
	QueueLen(ctx context.Context, roomID string) (int64, error)

	// ExistsInQueue reports whether id is currently in the queue.
	ExistsInQueue(ctx context.Context, roomID, id string) (bool, error)
}
