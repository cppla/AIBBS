package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/utils"
)

type rateLimiter struct {
	limiter *rate.Limiter
	expires time.Time
	mu      sync.Mutex
}

var (
	limiters   = map[string]*rateLimiter{}
	limitersMu sync.Mutex
)

// RateLimitMiddleware applies a simple IP based rate limiter using a token bucket.
func RateLimitMiddleware() gin.HandlerFunc {
	cfg := config.Get()
	r := rate.Every(time.Minute / time.Duration(max(cfg.RateLimitPerMinute, 1)))
	burst := max(cfg.RateLimitPerMinute/2, 1)

	return func(ctx *gin.Context) {
		ip := ctx.ClientIP()
		limiter := getLimiter(ip, r, burst)

		limiter.mu.Lock()
		allowed := limiter.limiter.Allow()
		limiter.mu.Unlock()

		if !allowed {
			utils.Error(ctx, 429, 42901, "rate limit exceeded")
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

func getLimiter(key string, limit rate.Limit, burst int) *rateLimiter {
	limitersMu.Lock()
	defer limitersMu.Unlock()

	cleanupExpiredLimitersLocked()

	if limiter, ok := limiters[key]; ok {
		limiter.expires = time.Now().Add(5 * time.Minute)
		return limiter
	}

	limiter := &rateLimiter{
		limiter: rate.NewLimiter(limit, burst),
		expires: time.Now().Add(5 * time.Minute),
	}
	limiters[key] = limiter
	return limiter
}

func cleanupExpiredLimitersLocked() {
	now := time.Now()
	for key, limiter := range limiters {
		if now.After(limiter.expires) {
			delete(limiters, key)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
