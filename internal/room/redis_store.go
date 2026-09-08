package room

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStoreOptions configures the Redis connection pool.
type RedisStoreOptions struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int           // default: 10
	DialTimeout  time.Duration // default: 5s
	ReadTimeout  time.Duration // default: 3s
	WriteTimeout time.Duration // default: 3s
}

// RedisStore implements Store using Redis ZSET structures.
// All active-session expiry scores are Unix millis.
// All queue scores are Unix millis (FIFO ordering).
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a RedisStore from a simple address string with sensible defaults.
func NewRedisStore(addr string) *RedisStore {
	return NewRedisStoreWithOptions(RedisStoreOptions{Addr: addr})
}

// NewRedisStoreWithOptions creates a RedisStore with full control over connection settings.
func NewRedisStoreWithOptions(opts RedisStoreOptions) *RedisStore {
	if opts.PoolSize <= 0 {
		opts.PoolSize = 10
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 5 * time.Second
	}
	if opts.ReadTimeout <= 0 {
		opts.ReadTimeout = 3 * time.Second
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = 3 * time.Second
	}

	c := redis.NewClient(&redis.Options{
		Addr:         opts.Addr,
		Password:     opts.Password,
		DB:           opts.DB,
		PoolSize:     opts.PoolSize,
		DialTimeout:  opts.DialTimeout,
		ReadTimeout:  opts.ReadTimeout,
		WriteTimeout: opts.WriteTimeout,
	})
	return &RedisStore{client: c}
}

// Key helpers — prefix is "owr:" (open waiting room).
func (s *RedisStore) queueKey(roomID string) string  { return fmt.Sprintf("owr:queue:%s", roomID) }
func (s *RedisStore) activeKey(roomID string) string { return fmt.Sprintf("owr:active:%s", roomID) }

// --- Active ---

// ActiveCount removes expired sessions then returns the count of valid active sessions.
func (s *RedisStore) ActiveCount(ctx context.Context, roomID string) (int64, error) {
	now := time.Now().UnixMilli()
	// Evict expired (score ≤ now) before counting.
	if err := s.client.ZRemRangeByScore(ctx, s.activeKey(roomID), "0", fmt.Sprintf("%d", now)).Err(); err != nil {
		return 0, fmt.Errorf("redis ActiveCount cleanup: %w", err)
	}
	cnt, err := s.client.ZCard(ctx, s.activeKey(roomID)).Result()
	return cnt, err
}

// IsActive reports whether id holds a non-expired active session.
func (s *RedisStore) IsActive(ctx context.Context, roomID, id string) (bool, error) {
	score, err := s.client.ZScore(ctx, s.activeKey(roomID), id).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis IsActive: %w", err)
	}
	return int64(score) > time.Now().UnixMilli(), nil
}

// AddActive registers id as active with expiry expiresAtMillis (Unix millis).
func (s *RedisStore) AddActive(ctx context.Context, roomID, id string, expiresAtMillis int64) error {
	err := s.client.ZAdd(ctx, s.activeKey(roomID), redis.Z{
		Score:  float64(expiresAtMillis),
		Member: id,
	}).Err()
	if err != nil {
		return fmt.Errorf("redis AddActive: %w", err)
	}
	return nil
}

// ExtendActive updates the expiry of an existing active session.
func (s *RedisStore) ExtendActive(ctx context.Context, roomID, id string, newExpiryMillis int64) error {
	err := s.client.ZAdd(ctx, s.activeKey(roomID), redis.Z{
		Score:  float64(newExpiryMillis),
		Member: id,
	}).Err()
	if err != nil {
		return fmt.Errorf("redis ExtendActive: %w", err)
	}
	return nil
}

// RemoveActive removes an active session.
func (s *RedisStore) RemoveActive(ctx context.Context, roomID, id string) error {
	if err := s.client.ZRem(ctx, s.activeKey(roomID), id).Err(); err != nil {
		return fmt.Errorf("redis RemoveActive: %w", err)
	}
	return nil
}

// CleanupExpiredActive removes sessions with score ≤ nowMillis.
func (s *RedisStore) CleanupExpiredActive(ctx context.Context, roomID string, nowMillis int64) (int64, error) {
	removed, err := s.client.ZRemRangeByScore(ctx, s.activeKey(roomID), "0", fmt.Sprintf("%d", nowMillis)).Result()
	if err != nil {
		return 0, fmt.Errorf("redis CleanupExpiredActive: %w", err)
	}
	return removed, nil
}

// --- Queue ---

// Enqueue adds item to the queue with ZADDNX (no-op if already present).
func (s *RedisStore) Enqueue(ctx context.Context, item QueueItem) (int64, int64, error) {
	if _, err := s.client.ZAddNX(ctx, s.queueKey(item.RoomID), redis.Z{
		Score:  float64(item.Score),
		Member: item.ID,
	}).Result(); err != nil {
		return 0, 0, fmt.Errorf("redis Enqueue ZAddNX: %w", err)
	}

	rank, err := s.client.ZRank(ctx, s.queueKey(item.RoomID), item.ID).Result()
	if err != nil {
		return 0, 0, fmt.Errorf("redis Enqueue ZRank: %w", err)
	}
	total, err := s.client.ZCard(ctx, s.queueKey(item.RoomID)).Result()
	if err != nil {
		return 0, 0, fmt.Errorf("redis Enqueue ZCard: %w", err)
	}
	return rank, total, nil
}

// GetPosition returns the zero-based rank of id in the queue.
func (s *RedisStore) GetPosition(ctx context.Context, roomID, id string) (int64, int64, bool, error) {
	rank, err := s.client.ZRank(ctx, s.queueKey(roomID), id).Result()
	if errors.Is(err, redis.Nil) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("redis GetPosition: %w", err)
	}
	total, err := s.client.ZCard(ctx, s.queueKey(roomID)).Result()
	if err != nil {
		return 0, 0, false, fmt.Errorf("redis GetPosition ZCard: %w", err)
	}
	return rank, total, true, nil
}

// PopForAdmission removes and returns up to n items using method "fifo" or "random".
//
// FIFO: ZPopMin — atomic single command.
// Random: uses a Lua script to atomically fetch a sample, shuffle, and remove.
func (s *RedisStore) PopForAdmission(ctx context.Context, roomID string, n int, method string) ([]QueueItem, error) {
	if method == "random" {
		return s.popRandom(ctx, roomID, n)
	}
	return s.popFIFO(ctx, roomID, n)
}

// popFIFO atomically pops n lowest-score items via ZPOPMIN.
func (s *RedisStore) popFIFO(ctx context.Context, roomID string, n int) ([]QueueItem, error) {
	res, err := s.client.ZPopMin(ctx, s.queueKey(roomID), int64(n)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis popFIFO: %w", err)
	}
	items := make([]QueueItem, 0, len(res))
	for _, z := range res {
		score := int64(z.Score)
		memberID, ok := z.Member.(string)
		if !ok {
			memberID = fmt.Sprintf("%v", z.Member)
		}
		items = append(items, QueueItem{
			ID:         memberID,
			RoomID:     roomID,
			Score:      score,
			EnqueuedAt: time.UnixMilli(score),
		})
	}
	return items, nil
}

// luaRandomPop atomically picks n random members from a ZSET and removes them.
// KEYS[1] = queue key, ARGV[1] = candidate pool size (n*5), ARGV[2] = n
var luaRandomPop = redis.NewScript(`
local key = KEYS[1]
local pool = tonumber(ARGV[1])
local n    = tonumber(ARGV[2])
local all  = redis.call('ZRANGE', key, 0, pool - 1)
if #all == 0 then return {} end
-- Fisher-Yates partial shuffle in Lua
math.randomseed(tonumber(redis.call('TIME')[1]))
for i = #all, 2, -1 do
  local j = math.random(i)
  all[i], all[j] = all[j], all[i]
end
local picked = {}
for i = 1, math.min(n, #all) do
  picked[i] = all[i]
  redis.call('ZREM', key, all[i])
end
return picked
`)

// popRandom uses a Lua script to atomically sample and remove n random items.
func (s *RedisStore) popRandom(ctx context.Context, roomID string, n int) ([]QueueItem, error) {
	pool := n * 5
	if pool < n {
		pool = n
	}

	res, err := luaRandomPop.Run(ctx, s.client, []string{s.queueKey(roomID)},
		pool, n,
	).StringSlice()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("redis popRandom: %w", err)
	}

	now := time.Now()
	items := make([]QueueItem, 0, len(res))
	for _, id := range res {
		// Shuffle candidates that were already fetched but not in ZRange result
		// use rand.Shuffle on the result slice for extra fairness.
		items = append(items, QueueItem{
			ID:         id,
			RoomID:     roomID,
			Score:      now.UnixMilli(),
			EnqueuedAt: now,
		})
	}
	rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] }) // #nosec G404 -- pseudo-random shuffle for non-crypto queue admission
	return items, nil
}

// RemoveFromQueue removes a single member from the queue.
func (s *RedisStore) RemoveFromQueue(ctx context.Context, roomID, id string) error {
	if err := s.client.ZRem(ctx, s.queueKey(roomID), id).Err(); err != nil {
		return fmt.Errorf("redis RemoveFromQueue: %w", err)
	}
	return nil
}

// QueueLen returns the number of items currently in the queue.
func (s *RedisStore) QueueLen(ctx context.Context, roomID string) (int64, error) {
	n, err := s.client.ZCard(ctx, s.queueKey(roomID)).Result()
	if err != nil {
		return 0, fmt.Errorf("redis QueueLen: %w", err)
	}
	return n, nil
}

// ExistsInQueue reports whether id is in the queue.
func (s *RedisStore) ExistsInQueue(ctx context.Context, roomID, id string) (bool, error) {
	_, err := s.client.ZRank(ctx, s.queueKey(roomID), id).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis ExistsInQueue: %w", err)
	}
	return true, nil
}
