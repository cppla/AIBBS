package utils

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: 3 * time.Second}

type ipAPIResp struct {
	IP       string `json:"ip"`
	Location string `json:"location"`
	Xad      string `json:"xad"`
}

// simple in-memory TTL cache
type cacheEntry struct {
	value     string
	expiresAt time.Time
}

var (
	ipCountryMu    sync.RWMutex
	ipCountryCache = make(map[string]cacheEntry)
	ipCountryTTL   = 24 * time.Hour
)

// NormalizeCountryName normalizes a country-like name by splitting on various dashes and spaces,
// returning the first segment (e.g., "中国–北京–北京" -> "中国", "美国 3COM公司企业网" -> "美国").
func NormalizeCountryName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	// Map various dash runes to a common '-'
	dashMapped := strings.Map(func(r rune) rune {
		switch r {
		case '-', '–', '—', '‑', '‒', '﹣', '－': // hyphen-minus, en dash, em dash, non-breaking hyphen, figure dash, small hyphen, fullwidth hyphen
			return '-'
		default:
			return r
		}
	}, s)
	// If we have a dash, take the first segment before '-'
	if idx := strings.IndexRune(dashMapped, '-'); idx >= 0 {
		return strings.TrimSpace(dashMapped[:idx])
	}
	// Otherwise, split by whitespace and take the first token
	toks := strings.Fields(dashMapped)
	if len(toks) > 0 {
		return strings.TrimSpace(toks[0])
	}
	return strings.TrimSpace(dashMapped)
}

// ExtractCountry from location string like "中国–成都–成都–双流区 联通"
func ExtractCountry(loc string) string {
	if loc == "" {
		return ""
	}
	return NormalizeCountryName(loc)
}

// GetIPLocation returns the full location string from the primary IP API provider.
func GetIPLocation(ctx context.Context, ip string) (string, error) {
	if ip == "" || IsPrivateIP(ip) {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.cloudcpp.com/ip/"+ip, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AIBBS/1.0 (compatible; AIBBSClient/1.0; +https://github.com/cppla/AIBBS)")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("ip api non-200")
	}
	var body ipAPIResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return strings.TrimSpace(body.Location), nil
}

// IsPrivateIP returns true for RFC1918 and loopback ranges.
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		return true
	}
	return false
}

// GetIPCountry returns the country for an IP (with in-memory and Redis caching).
// On error, returns empty country and error.
func GetIPCountry(ctx context.Context, ip string) (string, error) {
	if ip == "" || IsPrivateIP(ip) {
		return "", nil
	}
	// in-memory cache first
	if v, ok := cacheGet(ip); ok {
		return v, nil
	}
	// redis cache
	if v, ok := redisGet(ctx, ip); ok {
		cacheSet(ip, v)
		return v, nil
	}
	// remote fetch
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.cloudcpp.com/ip/"+ip, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AIBBS/1.0 (compatible; AIBBSClient/1.0; +https://github.com/cppla/AIBBS)")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("ip api non-200")
	}
	var body ipAPIResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	country := ExtractCountry(body.Location)
	if country != "" {
		cacheSet(ip, country)
		_ = redisSet(ctx, ip, country)
	}
	return country, nil
}

func cacheGet(ip string) (string, bool) {
	ipCountryMu.RLock()
	e, ok := ipCountryCache[ip]
	ipCountryMu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		ipCountryMu.Lock()
		delete(ipCountryCache, ip)
		ipCountryMu.Unlock()
		return "", false
	}
	return e.value, true
}

func cacheSet(ip, country string) {
	ipCountryMu.Lock()
	ipCountryCache[ip] = cacheEntry{value: country, expiresAt: time.Now().Add(ipCountryTTL)}
	ipCountryMu.Unlock()
}

func redisKey(ip string) string { return "ipcountry:" + ip }

func redisGet(ctx context.Context, ip string) (string, bool) {
	cli := GetRedis()
	ctx2, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	res := cli.Get(ctx2, redisKey(ip))
	if err := res.Err(); err != nil {
		return "", false
	}
	val, err := res.Result()
	if err != nil || val == "" {
		return "", false
	}
	return val, true
}

func redisSet(ctx context.Context, ip, country string) error {
	cli := GetRedis()
	ctx2, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	return cli.Set(ctx2, redisKey(ip), country, ipCountryTTL).Err()
}
