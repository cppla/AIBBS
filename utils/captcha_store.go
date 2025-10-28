package utils

import (
    "context"
    "time"

    "github.com/mojocn/base64Captcha"
)

// redisCaptchaStore implements base64Captcha.Store backed by Redis.
// It avoids per-instance memory state so captcha works behind load balancers.
type redisCaptchaStore struct {
    ttl time.Duration
}

func NewRedisCaptchaStore(ttl time.Duration) base64Captcha.Store {
    if ttl <= 0 {
        ttl = 10 * time.Minute
    }
    return &redisCaptchaStore{ttl: ttl}
}

func (s *redisCaptchaStore) key(id string) string {
    return "captcha:" + id
}

// Set stores the captcha value with TTL.
func (s *redisCaptchaStore) Set(id string, value string) error {
    rc := GetRedis()
    if rc == nil {
        return nil
    }
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    return rc.Set(ctx, s.key(id), value, s.ttl).Err()
}

// Get retrieves the value and optionally clears it.
func (s *redisCaptchaStore) Get(id string, clear bool) string {
    rc := GetRedis()
    if rc == nil {
        return ""
    }
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    key := s.key(id)
    if clear {
        // Prefer GETDEL (Redis >= 6.2)
        if v, err := rc.GetDel(ctx, key).Result(); err == nil {
            return v
        }
        // Fallback to Lua: GET then DEL atomically
        script := `local v=redis.call('GET', KEYS[1]); if v then redis.call('DEL', KEYS[1]); end; return v`
        if res, err := rc.Eval(ctx, script, []string{key}).Result(); err == nil {
            if res == nil {
                return ""
            }
            if s, ok := res.(string); ok {
                return s
            }
            return ""
        }
        return ""
    }
    v, err := rc.Get(ctx, key).Result()
    if err != nil {
        return ""
    }
    return v
}

// Verify compares answer and optionally clears it.
func (s *redisCaptchaStore) Verify(id, answer string, clear bool) bool {
    v := s.Get(id, clear)
    return v != "" && v == answer
}
