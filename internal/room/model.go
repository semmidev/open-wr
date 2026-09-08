// Package room defines the Store interface and data models for the waiting room queue.
package room

import "time"

// QueueItem represents a single entry in the waiting queue.
type QueueItem struct {
	ID         string    // unique visitor queue ID (UUID)
	RoomID     string    // which room this belongs to
	Score      int64     // Unix millis — used for FIFO ordering (lower = earlier)
	EnqueuedAt time.Time // wall-clock enqueue time
	IP         string    // visitor remote IP
	UserAgent  string    // visitor user-agent
}

// ActiveSession represents a session that has been admitted past the waiting room.
type ActiveSession struct {
	ID        string
	RoomID    string
	ExpiresAt time.Time // wall-clock session expiry
	IssuedAt  time.Time
}

// Stats is the snapshot of a room's current state.
type Stats struct {
	RoomID           string `json:"room_id"`
	ActiveCount      int64  `json:"active_count"`
	QueuedCount      int64  `json:"queued_count"`
	NewUsersPerMin   int64  `json:"new_users_per_minute"`
	TotalActiveLimit int64  `json:"total_active_limit"`
	QueueingMethod   string `json:"queueing_method"`
	IsQueueing       bool   `json:"is_queueing"`
}
