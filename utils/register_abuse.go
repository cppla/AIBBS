package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/cppla/aibbs/config"
)

func regKey(parts ...string) string {
	return "reg:" + join(parts, ":")
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += sep + parts[i]
	}
	return out
}

// RegistrationCooldownTry enforces a short cooldown between attempts per IP.
func RegistrationCooldownTry(ip string) bool {
	cfg := config.Get()
	sec := cfg.RegisterAttemptCooldownSec
	if sec <= 0 {
		return true
	}
	cli := GetRedis()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	key := regKey("cooldown", ip)
	ok, err := cli.SetNX(ctx, key, "1", time.Duration(sec)*time.Second).Result()
	if err != nil {
		return true
	} // fail-open
	return ok
}

// RegistrationDailyLimitCheck allows up to N successful registrations per day per IP.
func RegistrationDailyLimitCheck(ip string) bool {
	cfg := config.Get()
	limit := cfg.RegisterMaxPerIPPerDay
	if limit <= 0 {
		return true
	}
	cli := GetRedis()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	key := regKey("succday", ip, time.Now().Format("20060102"))
	n, err := cli.Get(ctx, key).Int()
	if err == redis.Nil {
		n = 0
	} else if err != nil {
		return true
	}
	return n < limit
}

// RegistrationDailyIncrement increments the success counter for today.
func RegistrationDailyIncrement(ip string) {
	cli := GetRedis()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	key := regKey("succday", ip, time.Now().Format("20060102"))
	if err := cli.Incr(ctx, key).Err(); err == nil {
		// set TTL to end of day
		ttl := time.Until(time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour))
		_ = cli.Expire(ctx, key, ttl).Err()
	}
}

// RegistrationFailRecord increments failure count per hour; returns current count.
func RegistrationFailRecord(ip string) int {
	cli := GetRedis()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	key := regKey("failhour", ip, time.Now().Format("2006010215"))
	n, err := cli.Incr(ctx, key).Result()
	if err != nil {
		return 0
	}
	_ = cli.Expire(ctx, key, time.Hour).Err()
	return int(n)
}

// RegistrationIsBanned checks temporary ban status for IP.
func RegistrationIsBanned(ip string) bool {
	cli := GetRedis()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	key := regKey("ban", ip)
	exists, err := cli.Exists(ctx, key).Result()
	if err != nil {
		return false
	}
	return exists > 0
}

// RegistrationBan sets a temporary ban for IP.
func RegistrationBan(ip string) {
	cfg := config.Get()
	minutes := cfg.RegisterTempBanMinutes
	if minutes <= 0 {
		minutes = 60
	}
	cli := GetRedis()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	key := regKey("ban", ip)
	_ = cli.Set(ctx, key, fmt.Sprintf("ban-%s", ip), time.Duration(minutes)*time.Minute).Err()
}
