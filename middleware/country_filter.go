package middleware

import (
	"context"
	"fmt"
	"html"
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/utils"
)

// CountryFilter enforces deny over allow based on client IP country.
// Behavior:
// - If DenyCountry contains the country: block
// - Else if AllowedCountry is non-empty and does NOT contain the country: block
// - Else allow
// - On lookup error: allow (fail-open)
func CountryFilter() gin.HandlerFunc {
	cfg := config.Get()
	denySet := toSet(cfg.DenyCountry)
	allowSet := toSet(cfg.AllowedCountry)
	haveAllow := len(allowSet) > 0

	return func(c *gin.Context) {
		ip := effectiveClientIP(c)
		// skip private IP (treat as allowed)
		if utils.IsPrivateIP(ip) {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		country, err := utils.GetIPCountry(ctx, ip)
		if err != nil {
			// fail open
			c.Next()
			return
		}
		if country == "" {
			c.Next()
			return
		}
		// normalize the country name for robust matching
		country = utils.NormalizeCountryName(country)
		// Deny has priority
		if _, bad := denySet[country]; bad {
			respondCountryBlocked(c, ip, country, 40301)
			return
		}
		if haveAllow {
			if _, ok := allowSet[country]; !ok {
				respondCountryBlocked(c, ip, country, 40302)
				return
			}
		}
		c.Next()
	}
}

func toSet(list []string) map[string]struct{} {
	m := make(map[string]struct{}, len(list))
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		v = utils.NormalizeCountryName(v)
		m[v] = struct{}{}
	}
	return m
}

func clientIP(c *gin.Context) string {
	// Fallback to gin's ClientIP with port stripping
	ip := c.ClientIP()
	if h, _, err := net.SplitHostPort(ip); err == nil {
		return h
	}
	return ip
}

// effectiveClientIP extracts the real visitor IP considering common proxy headers.
// Priority: CF-Connecting-IP > X-Real-IP > first of X-Forwarded-For > gin.ClientIP
func effectiveClientIP(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("CF-Connecting-IP")); v != "" {
		v = stripPort(v)
		if isValidPublicIP(v) {
			return v
		}
	}
	if v := strings.TrimSpace(c.GetHeader("X-Real-IP")); v != "" {
		v = stripPort(v)
		if isValidPublicIP(v) {
			return v
		}
	}
	if v := strings.TrimSpace(c.GetHeader("X-Forwarded-For")); v != "" {
		parts := strings.Split(v, ",")
		if len(parts) > 0 {
			cand := strings.TrimSpace(parts[0])
			cand = stripPort(cand)
			if isValidPublicIP(cand) {
				return cand
			}
		}
	}
	return clientIP(c)
}

func stripPort(ip string) string {
	if h, _, err := net.SplitHostPort(ip); err == nil {
		return h
	}
	return ip
}

func isValidPublicIP(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	if p.IsLoopback() || p.IsPrivate() {
		return false
	}
	return true
}

// respondCountryBlocked writes an HTML page for browser requests, otherwise JSON.
// The message displays the blocked visitor IP and detected country.
func respondCountryBlocked(c *gin.Context, ip string, country string, code int) {
	path := c.Request.URL.Path
	accept := c.GetHeader("Accept")
	// Build message in Chinese as requested, showing visitor IP
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "您的IP"
	}
	msg := fmt.Sprintf("当前 %s 不被允许访问", ip)

	isAPI := strings.HasPrefix(path, "/api/")
	wantsHTML := strings.Contains(strings.ToLower(accept), "text/html")

	if !isAPI && wantsHTML {
		// Simple friendly HTML page
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(403)
		esc := html.EscapeString(msg)
		cfg := config.Get()
		// detected location for display
		locText := ""
		if loc, err := utils.GetIPLocation(c.Request.Context(), ip); err == nil {
			locText = strings.TrimSpace(loc)
		}
		if locText == "" {
			// fallback to country when full location missing
			locText = strings.TrimSpace(country)
			if locText == "" {
				locText = "未知"
			}
		}
		locEsc := html.EscapeString(locText)
		// Build allowed/denied lists
		var allowedItems []string
		for _, v := range cfg.AllowedCountry {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			allowedItems = append(allowedItems, "<span class=\"tag allow\">"+html.EscapeString(v)+"</span>")
		}
		var deniedItems []string
		for _, v := range cfg.DenyCountry {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			deniedItems = append(deniedItems, "<span class=\"tag deny\">"+html.EscapeString(v)+"</span>")
		}
		allowedHTML := "<span class=\"muted\">（未配置，默认允许所有国家/地区，除非在禁止列表）</span>"
		if len(allowedItems) > 0 {
			allowedHTML = strings.Join(allowedItems, " ")
		}
		deniedHTML := "<span class=\"muted\">（无）</span>"
		if len(deniedItems) > 0 {
			deniedHTML = strings.Join(deniedItems, " ")
		}
		page := "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>访问受限</title><style>body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,Helvetica,Arial,sans-serif;background:#f7f7f9;margin:0;padding:0} .wrap{max-width:760px;margin:12vh auto} .card{background:#fff;border-radius:12px;box-shadow:0 2px 12px rgba(0,0,0,.08);padding:24px 20px;margin-bottom:16px} .card.rule{border-left:4px solid #0d6efd;background:#f8fbff;box-shadow:0 2px 14px rgba(13,110,253,.08)} h1{font-size:20px;margin:0 0 12px} p{color:#555;line-height:1.7;margin:0} .muted{color:#888;margin-top:12px;font-size:13px} .row{display:flex;gap:16px;flex-wrap:wrap;margin-top:8px} .col{flex:1 1 320px} .ttl{font-weight:600;margin-bottom:8px} .tag{display:inline-block;padding:2px 8px;border-radius:999px;font-size:12px;background:#eef1f5;color:#333;margin:2px 4px 2px 0} .tag.allow{background:#e7f7ef;color:#0a7d41} .tag.deny{background:#fdecec;color:#b21f1f}</style></head><body><div class=\"wrap\"><div class=\"card\"><h1>访问受限</h1><p>" + esc + "</p><p class=\"muted\">当前检测到的国家/地区（基于 IP 查询）：<strong>" + locEsc + "</strong></p><p class=\"muted\">定位来源：api.cloudcpp.com</p><p class=\"muted\">如有疑问，请联系站点管理员。</p></div><div class=\"card rule\"><div class=\"ttl\">访问规则</div><div class=\"row\"><div class=\"col\"><div class=\"ttl\">允许访问的国家/地区</div><div>" + allowedHTML + "</div></div><div class=\"col\"><div class=\"ttl\">不被允许访问的国家/地区</div><div>" + deniedHTML + "</div></div></div></div></div></body></html>"
		_, _ = c.Writer.Write([]byte(page))
		c.Abort()
		return
	}
	// Default JSON for API: include detected country and ip for debugging
	data := gin.H{"detected_country": strings.TrimSpace(country)}
	if strings.TrimSpace(ip) != "" {
		data["ip"] = strings.TrimSpace(ip)
	}
	utils.Respond(c, 403, code, msg, data)
	c.Abort()
}
