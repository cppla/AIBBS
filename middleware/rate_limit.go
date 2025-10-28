package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/utils"
)

// RateLimitMiddleware applies an IP-based per-minute limit using Redis counters.
// It is stateless across instances and safe behind load balancers.
func RateLimitMiddleware() gin.HandlerFunc {
	cfg := config.Get()
	limit := max(cfg.RateLimitPerMinute, 1)

	return func(ctx *gin.Context) {
		rc := utils.GetRedis()
		if rc == nil {
			// Fail-open if Redis not available
			ctx.Next()
			return
		}
	// Use the shared helper from country_filter.go in the same package
	ip := effectiveClientIP(ctx)
		key := fmt.Sprintf("ratelimit:%s:%s", ip, time.Now().Format("200601021504")) // per-minute window

		c, cancel := context.WithTimeout(ctx.Request.Context(), 500*time.Millisecond)
		defer cancel()
		n, err := rc.Incr(c, key).Result()
		if err == nil {
			if n == 1 {
				_ = rc.Expire(c, key, time.Minute).Err()
			}
			if n > int64(limit) {
				utils.Error(ctx, 429, 42901, "rate limit exceeded")
				ctx.Abort()
				return
			}
		}
		ctx.Next()
	}
}

func max(a, b int) int { if a > b { return a }; return b }
