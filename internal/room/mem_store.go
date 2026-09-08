package room

import (
	"context"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// memRoom holds the per-room state: a sorted FIFO queue and an active-session map.
// active maps visitor ID → expiry Unix millis.
type memRoom struct {
	mu     sync.RWMutex
	queue  []QueueItem      // sorted ascending by Score (Unix millis)
	qSet   map[string]bool  // O(1) existence check; true = in queue
	active map[string]int64 // id → expiry Unix millis
}

// MemStore is an in-process implementation of Store, suitable for single-instance
// deployments or local development.
type MemStore struct {
	mu    sync.RWMutex
	rooms map[string]*memRoom
}

// NewMemStore creates an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{rooms: make(map[string]*memRoom)}
}

// getOrCreate returns the memRoom for roomID, creating it if it doesn't exist.
func (s *MemStore) getOrCreate(roomID string) *memRoom {
	s.mu.RLock()
	r, ok := s.rooms[roomID]
	s.mu.RUnlock()
	if ok {
		return r
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check after acquiring write lock.
	if r, ok = s.rooms[roomID]; ok {
		return r
	}
	r = &memRoom{
		queue:  make([]QueueItem, 0, 64),
		qSet:   make(map[string]bool),
		active: make(map[string]int64),
	}
	s.rooms[roomID] = r
	return r
}

// --- Active ---

// ActiveCount returns the number of sessions whose expiry has not yet passed.
// It performs lazy eviction of expired entries.
func (s *MemStore) ActiveCount(ctx context.Context, roomID string) (int64, error) {
	r := s.getOrCreate(roomID)
	now := time.Now().UnixMilli()

	r.mu.Lock()
	defer r.mu.Unlock()

	var count int64
	for id, exp := range r.active {
		if now > exp {
			delete(r.active, id) // lazy eviction
		} else {
			count++
		}
	}
	return count, nil
}

// IsActive reports whether id holds a non-expired active session.
func (s *MemStore) IsActive(ctx context.Context, roomID, id string) (bool, error) {
	r := s.getOrCreate(roomID)

	r.mu.RLock()
	exp, ok := r.active[id]
	r.mu.RUnlock()

	if !ok {
		return false, nil
	}
	return time.Now().UnixMilli() <= exp, nil
}

// AddActive registers id as active with expiry expiresAtMillis (Unix millis).
func (s *MemStore) AddActive(ctx context.Context, roomID, id string, expiresAtMillis int64) error {
	r := s.getOrCreate(roomID)
	r.mu.Lock()
	r.active[id] = expiresAtMillis
	r.mu.Unlock()
	return nil
}

// ExtendActive updates the expiry of an existing session. No-op if id is not active.
func (s *MemStore) ExtendActive(ctx context.Context, roomID, id string, newExpiryMillis int64) error {
	r := s.getOrCreate(roomID)
	r.mu.Lock()
	if _, ok := r.active[id]; ok {
		r.active[id] = newExpiryMillis
	}
	r.mu.Unlock()
	return nil
}

// RemoveActive evicts id from the active set.
func (s *MemStore) RemoveActive(ctx context.Context, roomID, id string) error {
	r := s.getOrCreate(roomID)
	r.mu.Lock()
	delete(r.active, id)
	r.mu.Unlock()
	return nil
}

// CleanupExpiredActive removes all sessions whose expiry ≤ nowMillis.
func (s *MemStore) CleanupExpiredActive(ctx context.Context, roomID string, nowMillis int64) (int64, error) {
	r := s.getOrCreate(roomID)
	r.mu.Lock()
	defer r.mu.Unlock()

	var removed int64
	for id, exp := range r.active {
		if nowMillis >= exp {
			delete(r.active, id)
			removed++
		}
	}
	return removed, nil
}

// --- Queue ---

// Enqueue inserts item into the queue in Score-ascending order (FIFO by timestamp).
// If id already exists the current position is returned without re-insertion.
func (s *MemStore) Enqueue(ctx context.Context, item QueueItem) (int64, int64, error) {
	r := s.getOrCreate(item.RoomID)
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.qSet[item.ID] {
		// Already queued — find current position.
		pos := sort.Search(len(r.queue), func(i int) bool {
			return r.queue[i].ID == item.ID || r.queue[i].Score > item.Score
		})
		return int64(pos), int64(len(r.queue)), nil
	}

	// Insert in sorted position.
	idx := sort.Search(len(r.queue), func(i int) bool {
		return r.queue[i].Score >= item.Score
	})
	r.queue = append(r.queue, QueueItem{})
	copy(r.queue[idx+1:], r.queue[idx:])
	r.queue[idx] = item
	r.qSet[item.ID] = true

	return int64(idx), int64(len(r.queue)), nil
}

// GetPosition returns the zero-based position of id in the sorted queue.
func (s *MemStore) GetPosition(ctx context.Context, roomID, id string) (int64, int64, bool, error) {
	r := s.getOrCreate(roomID)
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.qSet[id] {
		return 0, int64(len(r.queue)), false, nil
	}
	for i, it := range r.queue {
		if it.ID == id {
			return int64(i), int64(len(r.queue)), true, nil
		}
	}
	// qSet inconsistency safety-net.
	return 0, int64(len(r.queue)), false, nil
}

// PopForAdmission removes and returns up to n items from the queue.
// method must be "fifo" or "random".
func (s *MemStore) PopForAdmission(ctx context.Context, roomID string, n int, method string) ([]QueueItem, error) {
	r := s.getOrCreate(roomID)
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.queue) == 0 || n <= 0 {
		return nil, nil
	}
	if n > len(r.queue) {
		n = len(r.queue)
	}

	var popped []QueueItem
	if method == "random" {
		popped = make([]QueueItem, 0, n)
		for i := 0; i < n; i++ {
			if len(r.queue) == 0 {
				break
			}
			idx := rand.Intn(len(r.queue)) // #nosec G404 -- pseudo-random sampling for non-crypto queue admission
			item := r.queue[idx]
			popped = append(popped, item)
			delete(r.qSet, item.ID)
			r.queue = append(r.queue[:idx], r.queue[idx+1:]...)
		}
	} else {
		// fifo: take from front
		popped = make([]QueueItem, n)
		copy(popped, r.queue[:n])
		for _, it := range popped {
			delete(r.qSet, it.ID)
		}
		r.queue = r.queue[n:]
	}

	return popped, nil
}

// RemoveFromQueue removes a single item by id from the queue.
func (s *MemStore) RemoveFromQueue(ctx context.Context, roomID, id string) error {
	r := s.getOrCreate(roomID)
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.qSet[id] {
		return nil
	}
	for i, it := range r.queue {
		if it.ID == id {
			r.queue = append(r.queue[:i], r.queue[i+1:]...)
			delete(r.qSet, id)
			return nil
		}
	}
	// Safety-net: remove from set if not found in slice.
	delete(r.qSet, id)
	return nil
}

// QueueLen returns the current queue length.
func (s *MemStore) QueueLen(ctx context.Context, roomID string) (int64, error) {
	r := s.getOrCreate(roomID)
	r.mu.RLock()
	n := int64(len(r.queue))
	r.mu.RUnlock()
	return n, nil
}

// ExistsInQueue reports whether id is currently in the queue.
func (s *MemStore) ExistsInQueue(ctx context.Context, roomID, id string) (bool, error) {
	r := s.getOrCreate(roomID)
	r.mu.RLock()
	ok := r.qSet[id]
	r.mu.RUnlock()
	return ok, nil
}
