package utils

import (
	"context"
	"encoding/json"
	"time"
)

const (
	// Default cache ttl set to 3600 seconds per requirement
	defaultCacheTTL = time.Hour
)

// CacheGetBytes returns cached bytes for a key from Redis.
func CacheGetBytes(key string) ([]byte, bool) {
	rc := GetRedis()
	if rc == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, err := rc.Get(ctx, key).Bytes()
	if err != nil {
		// debug: log miss or error once per call
		if Sugar != nil {
			Sugar.Debugf("cache get miss key=%s err=%v", key, err)
		}
		return nil, false
	}
	return b, true
}

// CacheSetBytes stores bytes with default TTL.
func CacheSetBytes(key string, b []byte, ttl time.Duration) {
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	rc := GetRedis()
	if rc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rc.Set(ctx, key, b, ttl).Err(); err != nil {
		if Sugar != nil {
			Sugar.Warnf("cache set failed key=%s err=%v", key, err)
		}
	}
}

// CacheSetJSON marshals v and stores JSON bytes.
func CacheSetJSON(key string, v interface{}, ttl time.Duration) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	CacheSetBytes(key, b, ttl)
}

// InvalidateByPrefix deletes keys that match the given prefix using SCAN.
func InvalidateByPrefix(prefix string) {
	rc := GetRedis()
	if rc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var cursor uint64
	for i := 0; i < 10; i++ { // limit rounds to avoid long loops
		keys, cur, err := rc.Scan(ctx, cursor, prefix+"*", 1000).Result()
		if err != nil {
			break
		}
		cursor = cur
		if len(keys) > 0 {
			// pipeline delete
			pipe := rc.Pipeline()
			for _, k := range keys {
				pipe.Del(ctx, k)
			}
			_, _ = pipe.Exec(ctx)
		}
		if cursor == 0 {
			break
		}
	}
}
