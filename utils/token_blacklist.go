package utils

import (
	"sync"
	"time"
)

// blacklistEntry keeps expiration metadata for a JWT token.
type blacklistEntry struct {
	expiresAt time.Time
}

var (
	blacklist   = map[string]blacklistEntry{}
	blacklistMu sync.RWMutex
)

// BlacklistToken stores a token in memory until expiration to support logout semantics.
func BlacklistToken(token string, expiresAt time.Time) {
	blacklistMu.Lock()
	defer blacklistMu.Unlock()
	blacklist[token] = blacklistEntry{expiresAt: expiresAt}
}

// IsTokenBlacklisted checks if a token was revoked before natural expiration.
func IsTokenBlacklisted(token string) bool {
	blacklistMu.RLock()
	entry, ok := blacklist[token]
	blacklistMu.RUnlock()
	if !ok {
		return false
	}

	if time.Now().After(entry.expiresAt) {
		blacklistMu.Lock()
		delete(blacklist, token)
		blacklistMu.Unlock()
		return false
	}

	return true
}
