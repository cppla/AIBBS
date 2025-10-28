package utils

import (
	"context"
	"crypto/rand"
	"math/big"
	"sync"
	"time"
)

// in-memory fallback store
type codeEntry struct {
	code      string
	expiresAt time.Time
}

var (
	codeStore   = map[string]codeEntry{}
	codeStoreMu sync.Mutex
)

// GenerateVerificationCode creates a numeric code with given length.
func GenerateVerificationCode(n int) string {
	if n <= 0 {
		n = 6
	}
	digits := make([]byte, n)
	for i := 0; i < n; i++ {
		// crypto/rand for better unpredictability
		v, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// fallback to time based modulo if crypto fails
			v = big.NewInt(time.Now().UnixNano() % 10)
		}
		digits[i] = byte('0' + v.Int64())
	}
	return string(digits)
}

func codeKey(email string) string {
	return "verify:email:" + email
}

// SaveCode stores a code for an email with TTL. Prefer Redis; fallback to memory.
func SaveCode(email, code string, ttl time.Duration) {
	if rc := GetRedis(); rc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rc.Set(ctx, codeKey(email), code, ttl).Err(); err == nil {
			return
		}
	}
	codeStoreMu.Lock()
	codeStore[email] = codeEntry{code: code, expiresAt: time.Now().Add(ttl)}
	codeStoreMu.Unlock()
}

// VerifyAndConsumeCode checks a code and consumes it if valid. Prefer Redis; fallback to memory.
func VerifyAndConsumeCode(email, code string) bool {
	if rc := GetRedis(); rc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		key := codeKey(email)
		// Prefer GETDEL (Redis >= 6.2)
		if val, err := rc.GetDel(ctx, key).Result(); err == nil {
			return val == code
		}
		// Fallback to atomic Lua: GET then DEL
		script := `local v=redis.call('GET', KEYS[1]); if v then redis.call('DEL', KEYS[1]); end; return v`
		if res, err := rc.Eval(ctx, script, []string{key}).Result(); err == nil {
			if res == nil {
				return false
			}
			if s, ok := res.(string); ok {
				return s == code
			}
			// unexpected type
			return false
		}
		// On Redis error (e.g., network), fall through to memory fallback
	}
	codeStoreMu.Lock()
	defer codeStoreMu.Unlock()
	entry, ok := codeStore[email]
	if !ok {
		return false
	}
	if time.Now().After(entry.expiresAt) {
		delete(codeStore, email)
		return false
	}
	if entry.code != code {
		return false
	}
	delete(codeStore, email)
	return true
}

// EmailCooldownTrySet sets a cooldown key for sending email code. Returns true if set, false if cooling down.
func EmailCooldownTrySet(email string, cooldown time.Duration) bool {
	if rc := GetRedis(); rc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		key := "cooldown:email:" + email
		// NX with TTL
		ok, _ := rc.SetNX(ctx, key, "1", cooldown).Result()
		return ok
	}
	// memory fallback
	key := "cooldown:email:mem:" + email
	codeStoreMu.Lock()
	defer codeStoreMu.Unlock()
	if entry, ok := codeStore[key]; ok && time.Now().Before(entry.expiresAt) {
		return false
	}
	codeStore[key] = codeEntry{code: "1", expiresAt: time.Now().Add(cooldown)}
	return true
}
