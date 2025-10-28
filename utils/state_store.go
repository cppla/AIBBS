package utils

import (
	"context"
	"sync"
	"time"
)

type stateEntry struct {
	expiresAt time.Time
}

var (
	stateStore   = map[string]stateEntry{}
	stateStoreMu sync.Mutex
)

// SaveState stores an OAuth state token with TTL to mitigate CSRF.
func SaveState(state string, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	// Prefer Redis for distributed consistency
	if rc := GetRedis(); rc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = rc.Set(ctx, "oauth:state:"+state, "1", ttl).Err()
		return
	}
	// Fallback to in-memory (single-instance only)
	stateStoreMu.Lock()
	stateStore[state] = stateEntry{expiresAt: time.Now().Add(ttl)}
	stateStoreMu.Unlock()
}

// ConsumeState validates and removes a state token.
func ConsumeState(state string) bool {
	// Prefer Redis: GETDEL to ensure single-use
	if rc := GetRedis(); rc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		key := "oauth:state:" + state
		if v, err := rc.GetDel(ctx, key).Result(); err == nil {
			return v != ""
		}
		// Fallback to Lua to attempt atomic get+del when GETDEL not available
		script := `local v=redis.call('GET', KEYS[1]); if v then redis.call('DEL', KEYS[1]); end; return v`
		if res, err := rc.Eval(ctx, script, []string{key}).Result(); err == nil {
			return res != nil
		}
		return false
	}
	// Fallback to in-memory
	stateStoreMu.Lock()
	entry, ok := stateStore[state]
	if ok {
		delete(stateStore, state)
	}
	stateStoreMu.Unlock()
	if !ok {
		return false
	}
	return time.Now().Before(entry.expiresAt)
}
