package utils

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cppla/aibbs/config"
)

var (
	redisClient *redis.Client
	redisOnce   sync.Once
)

// GetRedis returns a singleton Redis client based on loaded config.
func GetRedis() *redis.Client {
	redisOnce.Do(func() {
		cfg := config.Get()
		redisClient = redis.NewClient(&redis.Options{
			Addr:         net.JoinHostPort(cfg.RedisHost, strconv.Itoa(cfg.RedisPort)),
			Password:     cfg.RedisPassword,
			DB:           cfg.RedisDB,
			DialTimeout:  3 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
		})
		// Optional: ping to validate; ignore error to allow fallback paths
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = redisClient.Ping(ctx).Err()
	})
	return redisClient
}
