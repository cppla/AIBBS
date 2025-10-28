package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AppConfig holds environment driven configuration values.
// Sensitive data should never have defaults inside code and must be provided via env files or the environment.
type AppConfig struct {
	AppPort            string
	JWTSecret          string
	DatabaseURI        string
	DBHost             string
	DBPort             string
	DBUser             string
	DBPassword         string
	DBName             string
	GitHubClientID     string
	GitHubClientSecret string
	GoogleClientID     string
	GoogleClientSecret string
	TelegramBotToken   string
	SigninRewardPoints int
	RateLimitPerMinute int
	AllowedOrigins     []string
	OAuthRedirectBase  string
	// Country access control
	AllowedCountry []string
	DenyCountry    []string
	// Gin framework configuration
	GinMode string
	GinPath string
	// Footer configuration
	FooterCol1Title    string
	FooterCol1HTML     string
	FooterCol2Title    string
	FooterLink1Name    string
	FooterLink1URL     string
	FooterLink2Name    string
	FooterLink2URL     string
	FooterLink3Name    string
	FooterLink3URL     string
	FooterCol3Title    string
	FooterTelegramURL  string
	FooterEmailLink    string
	FooterBroadcastURL string
	// Notice bar configuration
	NoticeTitle string
	NoticeHTML  string
	// SMTP for email verification
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
	SMTPTLS      bool
	// Redis for caching/verification
	RedisHost     string
	RedisPort     int
	RedisDB       int
	RedisPassword string
	// Logging configuration
	LogLevel      string
	LogPath       string
	LogMaxSizeMB  int
	LogMaxBackups int
	LogMaxAgeDays int
	LogCompress   bool
	// Registration security
	RegisterCaptchaEnabled        bool
	RegisterMaxPerIPPerDay        int
	RegisterAttemptCooldownSec    int
	RegisterFailedMaxPerIPPerHour int
	RegisterTempBanMinutes        int
	// Admins
	AdminUsernames []string
}

var cfg AppConfig
var loaded bool

// Load loads the application configuration from environment variables. It should be called once during boot.
func Load() AppConfig {
	if loaded {
		return cfg
	}

	// .env loading is disabled; configuration is sourced from config/config.json and environment variables.

	// Precedence: config/config.json -> defaults -> environment variable overrides
	// 1) Try to load JSON config (supports both flat and nested grouped keys)
	_ = loadJSONConfig(filepath.Join("config", "config.json"), &cfg)

	// 2) Fill defaults for any zero values
	applyDefaults(&cfg)

	// 3) Override from environment variables when set
	applyEnvOverrides(&cfg)

	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET must be set in environment variables")
	}

	loaded = true
	return cfg
}

// Get returns the cached configuration, loading it if necessary.
func Get() AppConfig {
	if !loaded {
		return Load()
	}
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// loadJSONConfig reads JSON file into cfg if present. Returns error only for invalid JSON.
func loadJSONConfig(path string, out *AppConfig) error {
	f, err := os.Open(path)
	if err != nil {
		return nil // silently ignore missing file
	}
	defer f.Close()

	var raw map[string]any
	dec := json.NewDecoder(f)
	if err := dec.Decode(&raw); err != nil {
		return err
	}

	// Helper to read string/int/bool safely
	getString := func(m map[string]any, key string) string {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	getInt := func(m map[string]any, key string) int {
		if v, ok := m[key]; ok {
			switch t := v.(type) {
			case float64:
				return int(t)
			case int:
				return t
			case json.Number:
				i, _ := t.Int64()
				return int(i)
			}
		}
		return 0
	}
	getBool := func(m map[string]any, key string) bool {
		if v, ok := m[key]; ok {
			if b, ok := v.(bool); ok {
				return b
			}
		}
		return false
	}
	getStringSlice := func(m map[string]any, key string) []string {
		if v, ok := m[key]; ok {
			if arr, ok := v.([]any); ok {
				res := make([]string, 0, len(arr))
				for _, it := range arr {
					if s, ok := it.(string); ok {
						res = append(res, s)
					}
				}
				return res
			}
		}
		return nil
	}

	// Try grouped sections first
	if app, ok := raw["app"].(map[string]any); ok {
		out.AppPort = getString(app, "AppPort")
		out.JWTSecret = getString(app, "JWTSecret")
		if v := getInt(app, "RateLimitPerMinute"); v != 0 {
			out.RateLimitPerMinute = v
		}
		if v := getInt(app, "SigninRewardPoints"); v != 0 {
			out.SigninRewardPoints = v
		}
		if list := getStringSlice(app, "AllowedOrigins"); len(list) > 0 {
			out.AllowedOrigins = list
		}
		if list := getStringSlice(app, "AllowedCountry"); len(list) > 0 {
			out.AllowedCountry = list
		}
		if list := getStringSlice(app, "DenyCountry"); len(list) > 0 {
			out.DenyCountry = list
		}
		if v := getString(app, "OAuthRedirectBase"); v != "" {
			out.OAuthRedirectBase = v
		}
		// Support configuring admins under app section as App.AdminUsernames
		if list := getStringSlice(app, "AdminUsernames"); len(list) > 0 {
			out.AdminUsernames = list
		}
		// (removed) uploads self-destruct settings
	}

	// gin section (backward compatibility)
	if g, ok := raw["gin"].(map[string]any); ok {
		if v := getString(g, "Mode"); v != "" {
			out.GinMode = v
		}
		if v := getString(g, "LogPath"); v != "" {
			out.GinPath = v
		}
	}

	if dbs, ok := raw["database"].(map[string]any); ok {
		out.DatabaseURI = getString(dbs, "DatabaseURI")
		out.DBHost = getString(dbs, "DBHost")
		out.DBPort = getString(dbs, "DBPort")
		out.DBUser = getString(dbs, "DBUser")
		out.DBPassword = getString(dbs, "DBPassword")
		out.DBName = getString(dbs, "DBName")
	}

	if rds, ok := raw["redis"].(map[string]any); ok {
		out.RedisHost = getString(rds, "RedisHost")
		if v := getInt(rds, "RedisPort"); v != 0 {
			out.RedisPort = v
		}
		if v := getInt(rds, "RedisDB"); v != 0 {
			out.RedisDB = v
		}
		out.RedisPassword = getString(rds, "RedisPassword")
	}

	if oa, ok := raw["oauth"].(map[string]any); ok {
		out.GitHubClientID = getString(oa, "GitHubClientID")
		out.GitHubClientSecret = getString(oa, "GitHubClientSecret")
		out.GoogleClientID = getString(oa, "GoogleClientID")
		out.GoogleClientSecret = getString(oa, "GoogleClientSecret")
		out.TelegramBotToken = getString(oa, "TelegramBotToken")
	}

	if ft, ok := raw["footer"].(map[string]any); ok {
		out.FooterCol1Title = getString(ft, "FooterCol1Title")
		out.FooterCol1HTML = getString(ft, "FooterCol1HTML")
		out.FooterCol2Title = getString(ft, "FooterCol2Title")
		out.FooterLink1Name = getString(ft, "FooterLink1Name")
		out.FooterLink1URL = getString(ft, "FooterLink1URL")
		out.FooterLink2Name = getString(ft, "FooterLink2Name")
		out.FooterLink2URL = getString(ft, "FooterLink2URL")
		out.FooterLink3Name = getString(ft, "FooterLink3Name")
		out.FooterLink3URL = getString(ft, "FooterLink3URL")
		out.FooterCol3Title = getString(ft, "FooterCol3Title")
		out.FooterTelegramURL = getString(ft, "FooterTelegramURL")
		out.FooterEmailLink = getString(ft, "FooterEmailLink")
		out.FooterBroadcastURL = getString(ft, "FooterBroadcastURL")
	}

	if nt, ok := raw["notice"].(map[string]any); ok {
		out.NoticeTitle = getString(nt, "Title")
		out.NoticeHTML = getString(nt, "HTML")
	}

	// Admin section
	if adm, ok := raw["admin"].(map[string]any); ok {
		if list := getStringSlice(adm, "Usernames"); len(list) > 0 {
			out.AdminUsernames = list
		}
	}

	if sm, ok := raw["smtp"].(map[string]any); ok {
		out.SMTPHost = getString(sm, "SMTPHost")
		if v := getInt(sm, "SMTPPort"); v != 0 {
			out.SMTPPort = v
		}
		out.SMTPUsername = getString(sm, "SMTPUsername")
		out.SMTPPassword = getString(sm, "SMTPPassword")
		out.SMTPFrom = getString(sm, "SMTPFrom")
		out.SMTPFromName = getString(sm, "SMTPFromName")
		out.SMTPTLS = getBool(sm, "SMTPTLS")
	}

	// logging (grouped)
	if lg, ok := raw["log"].(map[string]any); ok {
		if v := getString(lg, "Level"); v != "" {
			out.LogLevel = v
		}
		if v := getString(lg, "Path"); v != "" {
			out.LogPath = v
		}
		// Gin settings under log
		if v := getString(lg, "GinMode"); v != "" {
			out.GinMode = v
		}
		if v := getString(lg, "GinPath"); v != "" {
			out.GinPath = v
		}
		if v := getInt(lg, "MaxSizeMB"); v != 0 {
			out.LogMaxSizeMB = v
		}
		if v := getInt(lg, "MaxBackups"); v != 0 {
			out.LogMaxBackups = v
		}
		if v := getInt(lg, "MaxAgeDays"); v != 0 {
			out.LogMaxAgeDays = v
		}
		out.LogCompress = getBool(lg, "Compress")
	}

	// registration section
	if rg, ok := raw["register"].(map[string]any); ok {
		if b, ok := rg["CaptchaEnabled"].(bool); ok {
			out.RegisterCaptchaEnabled = b
		}
		if v, ok := rg["MaxPerIPPerDay"]; ok {
			switch t := v.(type) {
			case float64:
				out.RegisterMaxPerIPPerDay = int(t)
			case int:
				out.RegisterMaxPerIPPerDay = t
			}
		}
		if v, ok := rg["AttemptCooldownSec"]; ok {
			switch t := v.(type) {
			case float64:
				out.RegisterAttemptCooldownSec = int(t)
			case int:
				out.RegisterAttemptCooldownSec = t
			}
		}
		if v, ok := rg["FailedMaxPerIPPerHour"]; ok {
			switch t := v.(type) {
			case float64:
				out.RegisterFailedMaxPerIPPerHour = int(t)
			case int:
				out.RegisterFailedMaxPerIPPerHour = t
			}
		}
		if v, ok := rg["TempBanMinutes"]; ok {
			switch t := v.(type) {
			case float64:
				out.RegisterTempBanMinutes = int(t)
			case int:
				out.RegisterTempBanMinutes = t
			}
		}
	}

	// Also support reading flat keys directly for backward compatibility
	if v, ok := raw["AppPort"]; ok && out.AppPort == "" {
		out.AppPort = v.(string)
	}
	if v, ok := raw["JWTSecret"]; ok && out.JWTSecret == "" {
		out.JWTSecret = v.(string)
	}
	if v, ok := raw["GinMode"]; ok && out.GinMode == "" {
		if s, ok := v.(string); ok {
			out.GinMode = s
		}
	}
	if v, ok := raw["GinPath"]; ok && out.GinPath == "" {
		if s, ok := v.(string); ok {
			out.GinPath = s
		}
	}
	// backward compatibility: flat key GinLogPath
	if v, ok := raw["GinLogPath"]; ok && out.GinPath == "" {
		if s, ok := v.(string); ok {
			out.GinPath = s
		}
	}
	if v, ok := raw["RateLimitPerMinute"]; ok && out.RateLimitPerMinute == 0 {
		if f, ok := v.(float64); ok {
			out.RateLimitPerMinute = int(f)
		}
	}
	if v, ok := raw["SigninRewardPoints"]; ok && out.SigninRewardPoints == 0 {
		if f, ok := v.(float64); ok {
			out.SigninRewardPoints = int(f)
		}
	}
	if v, ok := raw["AllowedOrigins"]; ok && len(out.AllowedOrigins) == 0 {
		if arr, ok := v.([]any); ok {
			for _, it := range arr {
				if s, ok := it.(string); ok {
					out.AllowedOrigins = append(out.AllowedOrigins, s)
				}
			}
		}
	}
	if v, ok := raw["AllowedCountry"]; ok && len(out.AllowedCountry) == 0 {
		if arr, ok := v.([]any); ok {
			for _, it := range arr {
				if s, ok := it.(string); ok {
					out.AllowedCountry = append(out.AllowedCountry, s)
				}
			}
		}
	}
	if v, ok := raw["DenyCountry"]; ok && len(out.DenyCountry) == 0 {
		if arr, ok := v.([]any); ok {
			for _, it := range arr {
				if s, ok := it.(string); ok {
					out.DenyCountry = append(out.DenyCountry, s)
				}
			}
		}
	}
	// flat AdminUsernames
	if v, ok := raw["AdminUsernames"]; ok && len(out.AdminUsernames) == 0 {
		if arr, ok := v.([]any); ok {
			for _, it := range arr {
				if s, ok := it.(string); ok {
					out.AdminUsernames = append(out.AdminUsernames, s)
				}
			}
		}
	}

	// (removed) flat uploads self-destruct keys
	if v, ok := raw["OAuthRedirectBase"]; ok && out.OAuthRedirectBase == "" {
		out.OAuthRedirectBase, _ = v.(string)
	}

	if v, ok := raw["DatabaseURI"]; ok && out.DatabaseURI == "" {
		out.DatabaseURI, _ = v.(string)
	}
	if v, ok := raw["DBHost"]; ok && out.DBHost == "" {
		out.DBHost, _ = v.(string)
	}
	if v, ok := raw["DBPort"]; ok && out.DBPort == "" {
		out.DBPort, _ = v.(string)
	}
	if v, ok := raw["DBUser"]; ok && out.DBUser == "" {
		out.DBUser, _ = v.(string)
	}
	if v, ok := raw["DBPassword"]; ok && out.DBPassword == "" {
		out.DBPassword, _ = v.(string)
	}
	if v, ok := raw["DBName"]; ok && out.DBName == "" {
		out.DBName, _ = v.(string)
	}

	if v, ok := raw["RedisHost"]; ok && out.RedisHost == "" {
		out.RedisHost, _ = v.(string)
	}
	if v, ok := raw["RedisPort"]; ok && out.RedisPort == 0 {
		if f, ok := v.(float64); ok {
			out.RedisPort = int(f)
		}
	}
	if v, ok := raw["RedisDB"]; ok && out.RedisDB == 0 {
		if f, ok := v.(float64); ok {
			out.RedisDB = int(f)
		}
	}
	if v, ok := raw["RedisPassword"]; ok && out.RedisPassword == "" {
		out.RedisPassword, _ = v.(string)
	}

	if v, ok := raw["GitHubClientID"]; ok && out.GitHubClientID == "" {
		out.GitHubClientID, _ = v.(string)
	}
	if v, ok := raw["GitHubClientSecret"]; ok && out.GitHubClientSecret == "" {
		out.GitHubClientSecret, _ = v.(string)
	}
	if v, ok := raw["GoogleClientID"]; ok && out.GoogleClientID == "" {
		out.GoogleClientID, _ = v.(string)
	}
	if v, ok := raw["GoogleClientSecret"]; ok && out.GoogleClientSecret == "" {
		out.GoogleClientSecret, _ = v.(string)
	}
	if v, ok := raw["TelegramBotToken"]; ok && out.TelegramBotToken == "" {
		out.TelegramBotToken, _ = v.(string)
	}

	if v, ok := raw["SMTPHost"]; ok && out.SMTPHost == "" {
		out.SMTPHost, _ = v.(string)
	}
	if v, ok := raw["SMTPPort"]; ok && out.SMTPPort == 0 {
		if f, ok := v.(float64); ok {
			out.SMTPPort = int(f)
		}
	}
	if v, ok := raw["SMTPUsername"]; ok && out.SMTPUsername == "" {
		out.SMTPUsername, _ = v.(string)
	}
	if v, ok := raw["SMTPPassword"]; ok && out.SMTPPassword == "" {
		out.SMTPPassword, _ = v.(string)
	}
	if v, ok := raw["SMTPFrom"]; ok && out.SMTPFrom == "" {
		out.SMTPFrom, _ = v.(string)
	}
	if v, ok := raw["SMTPFromName"]; ok && out.SMTPFromName == "" {
		out.SMTPFromName, _ = v.(string)
	}
	if v, ok := raw["SMTPTLS"]; ok {
		if b, ok := v.(bool); ok {
			out.SMTPTLS = b
		}
	}

	// logging (flat keys fallback)
	if v, ok := raw["LogLevel"]; ok && out.LogLevel == "" {
		if s, ok := v.(string); ok {
			out.LogLevel = s
		}
	}
	if v, ok := raw["LogPath"]; ok && out.LogPath == "" {
		if s, ok := v.(string); ok {
			out.LogPath = s
		}
	}
	if v, ok := raw["LogMaxSizeMB"]; ok && out.LogMaxSizeMB == 0 {
		if f, ok := v.(float64); ok {
			out.LogMaxSizeMB = int(f)
		}
	}
	if v, ok := raw["LogMaxBackups"]; ok && out.LogMaxBackups == 0 {
		if f, ok := v.(float64); ok {
			out.LogMaxBackups = int(f)
		}
	}
	if v, ok := raw["LogMaxAgeDays"]; ok && out.LogMaxAgeDays == 0 {
		if f, ok := v.(float64); ok {
			out.LogMaxAgeDays = int(f)
		}
	}
	if v, ok := raw["LogCompress"]; ok {
		if b, ok := v.(bool); ok {
			out.LogCompress = b
		}
	}

	// registration (flat keys)
	if v, ok := raw["RegisterCaptchaEnabled"]; ok {
		if b, ok := v.(bool); ok {
			out.RegisterCaptchaEnabled = b
		}

		// notice (flat keys)
		if v, ok := raw["NoticeTitle"]; ok && out.NoticeTitle == "" {
			if s, ok := v.(string); ok {
				out.NoticeTitle = s
			}
		}
		if v, ok := raw["NoticeHTML"]; ok && out.NoticeHTML == "" {
			if s, ok := v.(string); ok {
				out.NoticeHTML = s
			}
		}
	}
	if v, ok := raw["RegisterMaxPerIPPerDay"]; ok && out.RegisterMaxPerIPPerDay == 0 {
		if f, ok := v.(float64); ok {
			out.RegisterMaxPerIPPerDay = int(f)
		}
	}
	if v, ok := raw["RegisterAttemptCooldownSec"]; ok && out.RegisterAttemptCooldownSec == 0 {
		if f, ok := v.(float64); ok {
			out.RegisterAttemptCooldownSec = int(f)
		}
	}
	if v, ok := raw["RegisterFailedMaxPerIPPerHour"]; ok && out.RegisterFailedMaxPerIPPerHour == 0 {
		if f, ok := v.(float64); ok {
			out.RegisterFailedMaxPerIPPerHour = int(f)
		}
	}
	if v, ok := raw["RegisterTempBanMinutes"]; ok && out.RegisterTempBanMinutes == 0 {
		if f, ok := v.(float64); ok {
			out.RegisterTempBanMinutes = int(f)
		}
	}

	return nil
}

// applyDefaults sets sane defaults for zero-value fields.
func applyDefaults(c *AppConfig) {
	if c.AppPort == "" {
		c.AppPort = "8080"
	}
	if c.GinMode == "" {
		c.GinMode = "release"
	}
	if c.GinPath == "" {
		c.GinPath = "logs/go_gin.log"
	}
	if c.RateLimitPerMinute == 0 {
		c.RateLimitPerMinute = 60
	}
	if c.SigninRewardPoints == 0 {
		c.SigninRewardPoints = 10
	}
	if len(c.AllowedOrigins) == 0 {
		c.AllowedOrigins = []string{"*"}
	}
	if c.OAuthRedirectBase == "" {
		c.OAuthRedirectBase = "http://localhost:8080"
	}
	if c.DBHost == "" {
		c.DBHost = "127.0.0.1"
	}
	if c.DBPort == "" {
		c.DBPort = "3306"
	}
	if c.DBUser == "" {
		c.DBUser = "root"
	}
	if c.DBName == "" {
		c.DBName = "aibbs"
	}
	if c.SMTPPort == 0 {
		c.SMTPPort = 587
	}
	if c.RedisHost == "" {
		c.RedisHost = "127.0.0.1"
	}
	if c.RedisPort == 0 {
		c.RedisPort = 6379
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.LogMaxSizeMB == 0 {
		c.LogMaxSizeMB = 100
	}
	if c.LogMaxBackups == 0 {
		c.LogMaxBackups = 3
	}
	if c.LogMaxAgeDays == 0 {
		c.LogMaxAgeDays = 7
	}
	// Registration hardening defaults
	if c.RegisterMaxPerIPPerDay == 0 {
		c.RegisterMaxPerIPPerDay = 5
	}
	if c.RegisterAttemptCooldownSec == 0 {
		c.RegisterAttemptCooldownSec = 10
	}
	if c.RegisterFailedMaxPerIPPerHour == 0 {
		c.RegisterFailedMaxPerIPPerHour = 20
	}
	if c.RegisterTempBanMinutes == 0 {
		c.RegisterTempBanMinutes = 60
	}
	if c.NoticeTitle == "" {
		c.NoticeTitle = "公告"
	}
	if c.NoticeHTML == "" {
		c.NoticeHTML = "默认公告内容"
	}
	// (removed) uploads self-destruct defaults
}

// applyEnvOverrides maps known environment variables onto config values when present.
func applyEnvOverrides(c *AppConfig) {
	if v := getEnv("APP_PORT", ""); v != "" {
		c.AppPort = v
	}
	if v := getEnv("JWT_SECRET", ""); v != "" {
		c.JWTSecret = v
	}
	if v := getEnv("GIN_MODE", ""); v != "" {
		c.GinMode = v
	}
	if v := getEnv("GIN_LOG_PATH", ""); v != "" { // compatibility
		c.GinPath = v
	}
	if v := getEnv("GIN_PATH", ""); v != "" {
		c.GinPath = v
	}
	if v := getEnv("DATABASE_URI", ""); v != "" {
		c.DatabaseURI = v
	}
	if v := getEnv("DB_HOST", ""); v != "" {
		c.DBHost = v
	}
	if v := getEnv("DB_PORT", ""); v != "" {
		c.DBPort = v
	}
	if v := getEnv("DB_USER", ""); v != "" {
		c.DBUser = v
	}
	if v := getEnv("DB_PASSWORD", ""); v != "" {
		c.DBPassword = v
	}
	if v := getEnv("DB_NAME", ""); v != "" {
		c.DBName = v
	}
	if v := getEnv("GITHUB_CLIENT_ID", ""); v != "" {
		c.GitHubClientID = v
	}
	if v := getEnv("GITHUB_CLIENT_SECRET", ""); v != "" {
		c.GitHubClientSecret = v
	}
	if v := getEnv("GOOGLE_CLIENT_ID", ""); v != "" {
		c.GoogleClientID = v
	}
	if v := getEnv("GOOGLE_CLIENT_SECRET", ""); v != "" {
		c.GoogleClientSecret = v
	}
	if v := getEnv("TELEGRAM_BOT_TOKEN", ""); v != "" {
		c.TelegramBotToken = v
	}
	if v := getEnv("SIGNIN_REWARD", ""); v != "" {
		c.SigninRewardPoints = mustParseInt(v)
	}
	if v := getEnv("RATE_LIMIT_PER_MINUTE", ""); v != "" {
		c.RateLimitPerMinute = mustParseInt(v)
	}
	if v := getEnv("CORS_ALLOWED_ORIGINS", ""); v != "" {
		c.AllowedOrigins = readListEnv("CORS_ALLOWED_ORIGINS", c.AllowedOrigins)
	}
	if v := getEnv("ALLOWED_COUNTRY", ""); v != "" {
		c.AllowedCountry = readListEnv("ALLOWED_COUNTRY", c.AllowedCountry)
	}
	if v := getEnv("DENY_COUNTRY", ""); v != "" {
		c.DenyCountry = readListEnv("DENY_COUNTRY", c.DenyCountry)
	}
	if v := getEnv("OAUTH_REDIRECT_BASE_URL", ""); v != "" {
		c.OAuthRedirectBase = v
	}
	if v := getEnv("FOOTER_COL1_TITLE", ""); v != "" {
		c.FooterCol1Title = v
	}
	if v := getEnv("FOOTER_COL1_HTML", ""); v != "" {
		c.FooterCol1HTML = v
	}
	if v := getEnv("FOOTER_COL2_TITLE", ""); v != "" {
		c.FooterCol2Title = v
	}
	if v := getEnv("FOOTER_LINK1_NAME", ""); v != "" {
		c.FooterLink1Name = v
	}
	if v := getEnv("FOOTER_LINK1_URL", ""); v != "" {
		c.FooterLink1URL = v
	}
	if v := getEnv("FOOTER_LINK2_NAME", ""); v != "" {
		c.FooterLink2Name = v
	}
	if v := getEnv("FOOTER_LINK2_URL", ""); v != "" {
		c.FooterLink2URL = v
	}
	if v := getEnv("FOOTER_LINK3_NAME", ""); v != "" {
		c.FooterLink3Name = v
	}
	if v := getEnv("FOOTER_LINK3_URL", ""); v != "" {
		c.FooterLink3URL = v
	}
	if v := getEnv("FOOTER_COL3_TITLE", ""); v != "" {
		c.FooterCol3Title = v
	}
	if v := getEnv("FOOTER_TELEGRAM_URL", ""); v != "" {
		c.FooterTelegramURL = v
	}
	if v := getEnv("FOOTER_EMAIL_LINK", ""); v != "" {
		c.FooterEmailLink = v
	}
	if v := getEnv("FOOTER_BROADCAST_URL", ""); v != "" {
		c.FooterBroadcastURL = v
	}
	if v := getEnv("SMTP_HOST", ""); v != "" {
		c.SMTPHost = v
	}
	if v := getEnv("SMTP_PORT", ""); v != "" {
		c.SMTPPort = mustParseInt(v)
	}
	if v := getEnv("SMTP_USERNAME", ""); v != "" {
		c.SMTPUsername = v
	}
	if v := getEnv("SMTP_PASSWORD", ""); v != "" {
		c.SMTPPassword = v
	}
	if v := getEnv("SMTP_FROM", ""); v != "" {
		c.SMTPFrom = v
	}
	if v := getEnv("SMTP_FROM_NAME", ""); v != "" {
		c.SMTPFromName = v
	}
	if v := getEnv("SMTP_TLS", ""); v != "" {
		c.SMTPTLS = v == "true"
	}
	if v := getEnv("REDIS_HOST", ""); v != "" {
		c.RedisHost = v
	}
	if v := getEnv("REDIS_PORT", ""); v != "" {
		c.RedisPort = mustParseInt(v)
	}
	if v := getEnv("REDIS_DB", ""); v != "" {
		c.RedisDB = mustParseInt(v)
	}
	if v := getEnv("REDIS_PASSWORD", ""); v != "" {
		c.RedisPassword = v
	}
	// Logging env overrides
	if v := getEnv("LOG_LEVEL", ""); v != "" {
		c.LogLevel = v
	}
	if v := getEnv("LOG_PATH", ""); v != "" {
		c.LogPath = v
	}
	if v := getEnv("LOG_MAX_SIZE_MB", ""); v != "" {
		c.LogMaxSizeMB = mustParseInt(v)
	}
	if v := getEnv("LOG_MAX_BACKUPS", ""); v != "" {
		c.LogMaxBackups = mustParseInt(v)
	}
	if v := getEnv("LOG_MAX_AGE_DAYS", ""); v != "" {
		c.LogMaxAgeDays = mustParseInt(v)
	}
	if v := getEnv("LOG_COMPRESS", ""); v != "" {
		c.LogCompress = v == "true"
	}
	// Registration env overrides
	if v := getEnv("REGISTER_CAPTCHA_ENABLED", ""); v != "" {
		c.RegisterCaptchaEnabled = v == "true"
	}
	if v := getEnv("REGISTER_MAX_PER_IP_PER_DAY", ""); v != "" {
		c.RegisterMaxPerIPPerDay = mustParseInt(v)
	}
	if v := getEnv("REGISTER_ATTEMPT_COOLDOWN_SEC", ""); v != "" {
		c.RegisterAttemptCooldownSec = mustParseInt(v)
	}
	if v := getEnv("REGISTER_FAILED_MAX_PER_IP_PER_HOUR", ""); v != "" {
		c.RegisterFailedMaxPerIPPerHour = mustParseInt(v)
	}
	if v := getEnv("REGISTER_TEMP_BAN_MINUTES", ""); v != "" {
		c.RegisterTempBanMinutes = mustParseInt(v)
	}
	if v := getEnv("NOTICE_TITLE", ""); v != "" {
		c.NoticeTitle = v
	}
	if v := getEnv("NOTICE_HTML", ""); v != "" {
		c.NoticeHTML = v
	}
	// (removed) uploads self-destruct env overrides
}

func mustParseInt(val string) int {
	i, err := strconv.Atoi(val)
	if err != nil {
		log.Fatalf("invalid integer value %s: %v", val, err)
	}
	return i
}

func readListEnv(key string, defaults []string) []string {
	if raw := os.Getenv(key); raw != "" {
		return splitAndTrim(raw)
	}
	return defaults
}

func splitAndTrim(raw string) []string {
	items := []string{}
	for _, item := range splitByComma(raw) {
		trimmed := trimSpace(item)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func splitByComma(s string) []string {
	return strings.Split(s, ",")
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
