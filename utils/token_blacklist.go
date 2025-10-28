package utils

import (
	"context"
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
	// Prefer Redis: key with TTL until token expiration
	if rc := GetRedis(); rc != nil {
		ttl := time.Until(expiresAt)
		if ttl <= 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = rc.Set(ctx, "jwt:blacklist:"+token, "1", ttl).Err()
		return
	}
	// Fallback to in-memory
	blacklistMu.Lock()
	blacklist[token] = blacklistEntry{expiresAt: expiresAt}
	blacklistMu.Unlock()
}

// IsTokenBlacklisted checks if a token was revoked before natural expiration.
func IsTokenBlacklisted(token string) bool {
	// Prefer Redis
	if rc := GetRedis(); rc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		n, err := rc.Exists(ctx, "jwt:blacklist:"+token).Result()
		if err == nil {
			return n > 0
		}
		// On Redis error, fail closed? Choose to fail-open to avoid accidental lockout
		return false
	}
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
