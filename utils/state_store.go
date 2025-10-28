package utils

import (
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
	stateStoreMu.Lock()
	defer stateStoreMu.Unlock()
	stateStore[state] = stateEntry{expiresAt: time.Now().Add(ttl)}
}

// ConsumeState validates and removes a state token.
func ConsumeState(state string) bool {
	stateStoreMu.Lock()
	defer stateStoreMu.Unlock()

	entry, ok := stateStore[state]
	if !ok {
		return false
	}

	delete(stateStore, state)

	return time.Now().Before(entry.expiresAt)
}
