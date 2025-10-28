package controllers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"

	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/models"
	"github.com/cppla/aibbs/utils"
)

// AuthController handles authentication related endpoints including local and third-party providers.
type AuthController struct {
	db *gorm.DB
}

// ListUsers returns paginated users including register IP
func (a *AuthController) ListUsers(ctx *gin.Context) {
	var users []models.User
	var total int64

	page, pageSize := 1, 10
	if v := strings.TrimSpace(ctx.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := strings.TrimSpace(ctx.Query("page_size")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			pageSize = n
		}
	}

	if err := a.db.Model(&models.User{}).Count(&total).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50000, "failed to count users")
		return
	}

	if err := a.db.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50001, "failed to retrieve users")
		return
	}

	utils.Success(ctx, gin.H{
		"items": users,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": int((total + int64(pageSize) - 1) / int64(pageSize)),
		},
	})
}

// GetUserPublic returns public user info by ID
func (a *AuthController) GetUserPublic(ctx *gin.Context) {
	idStr := strings.TrimSpace(ctx.Param("id"))
	if idStr == "" {
		utils.Error(ctx, http.StatusBadRequest, 40050, "missing user id")
		return
	}
	// try cache first
	if b, ok := utils.CacheGetBytes("cache:user:public:" + idStr); ok {
		ctx.Data(http.StatusOK, "application/json", b)
		return
	}
	var user models.User
	if err := a.db.First(&user, idStr).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.Error(ctx, http.StatusNotFound, 40410, "user not found")
			return
		}
		utils.Error(ctx, http.StatusInternalServerError, 50050, "failed to get user")
		return
	}
	payload := sanitizeUserResponse(user)
	// cache wrapper for consistency
	wrapper := struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}{Code: 0, Message: "success", Data: payload}
	utils.CacheSetJSON("cache:user:public:"+idStr, wrapper, time.Hour)
	utils.Success(ctx, payload)
}

// GetUserPublicByUsername returns public user info by username
func (a *AuthController) GetUserPublicByUsername(ctx *gin.Context) {
	uname := strings.TrimSpace(ctx.Param("username"))
	if uname == "" {
		utils.Error(ctx, http.StatusBadRequest, 40051, "missing username")
		return
	}
	// try cache first
	if b, ok := utils.CacheGetBytes("cache:user:public:uname:" + uname); ok {
		ctx.Data(http.StatusOK, "application/json", b)
		return
	}
	var user models.User
	if err := a.db.Where("username = ?", uname).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.Error(ctx, http.StatusNotFound, 40411, "user not found")
			return
		}
		utils.Error(ctx, http.StatusInternalServerError, 50051, "failed to get user")
		return
	}
	payload := sanitizeUserResponse(user)
	wrapper := struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}{Code: 0, Message: "success", Data: payload}
	utils.CacheSetJSON("cache:user:public:uname:"+uname, wrapper, time.Hour)
	utils.Success(ctx, payload)
}

type telegramLoginRequest struct {
	ID        string `json:"id" binding:"required"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	PhotoURL  string `json:"photo_url"`
	AuthDate  int64  `json:"auth_date" binding:"required"`
	Hash      string `json:"hash" binding:"required"`
}

// NewAuthController creates an AuthController.
func NewAuthController(db *gorm.DB) *AuthController {
	return &AuthController{db: db}
}

// Register handles local account registration with bcrypt hashing.
func (a *AuthController) Register(ctx *gin.Context) {
	type request struct {
		Username      string `json:"username" binding:"required,min=3,max=64"`
		Email         string `json:"email"`
		Password      string `json:"password" binding:"required,min=6"`
		Confirm       string `json:"confirm"`
		Code          string `json:"code"`
		CaptchaID     string `json:"captcha_id"`
		CaptchaAnswer string `json:"captcha_answer"`
	}

	var req request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40001, "invalid request payload")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		utils.Error(ctx, http.StatusBadRequest, 40002, "用户名不能为空")
		return
	}
	// Username: 2-15, Chinese/English letters/digits and '-'
	if l := len([]rune(req.Username)); l < 2 || l > 15 {
		utils.Error(ctx, http.StatusBadRequest, 40002, "用户名长度需为2-15个字符")
		return
	}
	if !validUsername(req.Username) {
		utils.Error(ctx, http.StatusBadRequest, 40002, "用户名仅允许中文、英文、数字及 '-'")
		return
	}

	var existing models.User
	if err := a.db.Where("username = ?", req.Username).First(&existing).Error; err == nil {
		utils.Error(ctx, http.StatusConflict, 40901, "username already exists")
		return
	}

	// Password: 6-18 and only a-z A-Z 0-9 - _ .
	if req.Password != req.Confirm {
		utils.Error(ctx, http.StatusBadRequest, 40002, "两次输入的密码不一致")
		return
	}
	if len(req.Password) < 6 || len(req.Password) > 18 || !validPassword(req.Password) {
		utils.Error(ctx, http.StatusBadRequest, 40002, "密码需为6-18位，且仅包含字母、数字和 -_.")
		return
	}

	// Captcha is verified at SendEmailCode stage when enabled; no need to verify here to avoid二次验证

	// Email code verification
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Code) == "" {
		utils.Error(ctx, http.StatusBadRequest, 40002, "邮箱与验证码均为必填")
		return
	}
	if !utils.VerifyAndConsumeCode(strings.TrimSpace(req.Email), strings.TrimSpace(req.Code)) {
		utils.Error(ctx, http.StatusBadRequest, 40002, "验证码无效或已过期")
		return
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50001, "failed to hash password")
		return
	}

	// Anti-abuse: cooldown, per-IP daily limit, ban check
	ip := ctx.ClientIP()
	if utils.RegistrationIsBanned(ip) {
		utils.Error(ctx, http.StatusTooManyRequests, 42920, "当前 IP 已被临时限制，请稍后再试")
		return
	}
	if !utils.RegistrationCooldownTry(ip) {
		utils.Error(ctx, http.StatusTooManyRequests, 42910, "请求过于频繁，请稍后再试")
		return
	}
	if !utils.RegistrationDailyLimitCheck(ip) {
		utils.Error(ctx, http.StatusTooManyRequests, 42921, "今日注册次数已达上限")
		return
	}

	user := models.User{
		Username:     req.Username,
		Email:        strings.TrimSpace(req.Email),
		PasswordHash: hash,
		RegisterIP:   ip,
	}

	if err := a.db.Create(&user).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50002, "failed to create user")
		// record failure and maybe ban
		fails := utils.RegistrationFailRecord(ip)
		if fails >= max(config.Get().RegisterFailedMaxPerIPPerHour, 1) {
			utils.RegistrationBan(ip)
		}
		return
	}

	// No-op: we no longer use last_login_at for daily active metrics

	// record success for per-day limit
	utils.RegistrationDailyIncrement(ip)

	token, err := utils.GenerateToken(user.ID, user.Username, 72*time.Hour)
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50003, "failed to generate token")
		return
	}

	utils.Success(ctx, gin.H{
		"token": token,
		"user":  sanitizeUserResponseWithAdmin(user),
	})
}

// Captcha returns a fresh captcha id and base64 image (data URI)
func (a *AuthController) Captcha(ctx *gin.Context) {
	id, b64, err := utils.GenerateCaptcha()
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50060, "生成验证码失败")
		return
	}
	utils.Success(ctx, gin.H{"id": id, "image": b64})
}

// CaptchaVerify checks captcha without consuming it, used for client-side blur validation
func (a *AuthController) CaptchaVerify(ctx *gin.Context) {
	var req struct {
		ID     string `json:"captcha_id"`
		Answer string `json:"captcha_answer"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40061, "无效请求")
		return
	}
	ok := utils.VerifyCaptchaNoConsume(strings.TrimSpace(req.ID), strings.TrimSpace(req.Answer))
	if !ok {
		utils.Error(ctx, http.StatusBadRequest, 40062, "验证码不匹配")
		return
	}
	utils.Success(ctx, gin.H{"ok": true})
}

// SendEmailCode sends a verification code to user's email.
func (a *AuthController) SendEmailCode(ctx *gin.Context) {
	var req struct {
		Email         string `json:"email" binding:"required"`
		CaptchaID     string `json:"captcha_id"`
		CaptchaAnswer string `json:"captcha_answer"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40040, "无效的请求")
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		utils.Error(ctx, http.StatusBadRequest, 40041, "邮箱不能为空")
		return
	}
	// When enabled, captcha must be verified BEFORE sending email code
	if config.Get().RegisterCaptchaEnabled {
		if !utils.VerifyCaptcha(strings.TrimSpace(req.CaptchaID), strings.TrimSpace(req.CaptchaAnswer)) {
			utils.Error(ctx, http.StatusBadRequest, 40042, "验证码错误或已过期")
			return
		}
	}
	// basic cooldown: per-email 60s
	if !utils.EmailCooldownTrySet(email, 60*time.Second) {
		utils.Error(ctx, http.StatusTooManyRequests, 42910, "请求过于频繁，请稍后再试")
		return
	}
	// generate and send
	code := utils.GenerateVerificationCode(6)
	subject := "AIBBS 注册验证码"
	body := fmt.Sprintf("您的验证码是：%s\n10分钟内有效。", code)
	// 快速失败保护（整体接口在反代层仍可设置全局超时）
	if err := utils.SendMail(email, subject, body); err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50040, "验证码发送失败，请稍后重试")
		return
	}
	// 邮件发送成功后再保存验证码，避免无效验证码堆积
	utils.SaveCode(email, code, 10*time.Minute)
	utils.Success(ctx, gin.H{"message": "验证码已发送"})
}

// Helpers for validation
func validUsername(s string) bool {
	// Allow Chinese, letters, digits and '-'
	for _, r := range s {
		if r == '-' {
			continue
		}
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		// Chinese range (basic CJK)
		if r >= 0x4E00 && r <= 0x9FFF {
			continue
		}
		return false
	}
	return true
}

func validPassword(s string) bool {
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// Login verifies user credentials and issues a JWT.
func (a *AuthController) Login(ctx *gin.Context) {
	type request struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	var req request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40003, "invalid request payload")
		return
	}

	var user models.User
	if err := a.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		utils.Error(ctx, http.StatusUnauthorized, 40106, "invalid username or password")
		return
	}

	if !utils.CheckPassword(user.PasswordHash, req.Password) {
		utils.Error(ctx, http.StatusUnauthorized, 40106, "invalid username or password")
		return
	}

	// No-op: we no longer use last_login_at for daily active metrics

	token, err := utils.GenerateToken(user.ID, user.Username, 72*time.Hour)
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50004, "failed to generate token")
		return
	}

	utils.Success(ctx, gin.H{
		"token": token,
		"user":  sanitizeUserResponseWithAdmin(user),
	})
}

// Logout invalidates the token by blacklisting it until expiration.
func (a *AuthController) Logout(ctx *gin.Context) {
	authHeader := ctx.GetHeader("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		utils.Error(ctx, http.StatusUnauthorized, 40107, "invalid authorization header")
		return
	}

	token := strings.TrimSpace(parts[1])
	claims, err := utils.ParseToken(token)
	if err != nil {
		utils.Error(ctx, http.StatusUnauthorized, 40105, "invalid token")
		return
	}

	expiresAt := time.Now().Add(72 * time.Hour)
	if claims.RegisteredClaims.ExpiresAt != nil {
		expiresAt = claims.RegisteredClaims.ExpiresAt.Time
	}

	utils.BlacklistToken(token, expiresAt)
	utils.Success(ctx, gin.H{"message": "logged out"})
}

// OAuthRedirect generates a provider-specific authorization URL.
func (a *AuthController) OAuthRedirect(ctx *gin.Context) {
	provider := ctx.Param("provider")
	cfg, err := a.oauthConfig(provider)
	if err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40004, err.Error())
		return
	}

	state := uuid.NewString()
	utils.SaveState(state, 10*time.Minute)

	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
	utils.Success(ctx, gin.H{"authorization_url": url, "state": state})
}

// OAuthCallback exchanges the authorization code for a user identity and issues a JWT.
func (a *AuthController) OAuthCallback(ctx *gin.Context) {
	provider := ctx.Param("provider")
	code := ctx.Query("code")
	state := ctx.Query("state")

	if code == "" || state == "" {
		utils.Error(ctx, http.StatusBadRequest, 40005, "missing code or state")
		return
	}

	if !utils.ConsumeState(state) {
		utils.Error(ctx, http.StatusBadRequest, 40006, "invalid or expired state")
		return
	}

	cfg, err := a.oauthConfig(provider)
	if err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40004, err.Error())
		return
	}

	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40007, "failed to exchange code")
		return
	}

	userInfo, err := a.fetchOAuthUser(provider, token)
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50005, err.Error())
		return
	}

	user, err := a.findOrCreateOAuthUser(provider, userInfo)
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50006, "failed to persist user")
		return
	}

	// No-op: we no longer use last_login_at for daily active metrics

	jwtToken, err := utils.GenerateToken(user.ID, user.Username, 72*time.Hour)
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50004, "failed to generate token")
		return
	}

	utils.Success(ctx, gin.H{"token": jwtToken, "user": sanitizeUserResponseWithAdmin(*user)})
}

// TelegramLogin handles authentication via Telegram login widget.
func (a *AuthController) TelegramLogin(ctx *gin.Context) {
	var req telegramLoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40008, "invalid request payload")
		return
	}

	cfg := config.Get()
	if !verifyTelegramSignature(cfg.TelegramBotToken, req) {
		utils.Error(ctx, http.StatusUnauthorized, 40108, "invalid telegram signature")
		return
	}

	authTime := time.Unix(req.AuthDate, 0)
	if time.Since(authTime) > 5*time.Minute {
		utils.Error(ctx, http.StatusUnauthorized, 40109, "telegram login expired")
		return
	}

	displayName := strings.TrimSpace(req.FirstName + " " + req.LastName)
	if displayName == "" {
		displayName = req.Username
	}

	userInfo := oauthUser{
		ID:          req.ID,
		Username:    req.Username,
		DisplayName: displayName,
		Email:       "",
		AvatarURL:   req.PhotoURL,
	}

	user, err := a.findOrCreateOAuthUser("telegram", &userInfo)
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50006, "failed to persist user")
		return
	}

	// No-op: we no longer use last_login_at for daily active metrics

	token, err := utils.GenerateToken(user.ID, user.Username, 72*time.Hour)
	if err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50004, "failed to generate token")
		return
	}

	utils.Success(ctx, gin.H{"token": token, "user": sanitizeUserResponseWithAdmin(*user)})
}

// Me returns the current authenticated user's information.
func (a *AuthController) Me(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		utils.Error(ctx, http.StatusUnauthorized, 40108, "unauthorized")
		return
	}

	var user models.User
	if err := a.db.First(&user, userID).Error; err != nil {
		utils.Error(ctx, http.StatusNotFound, 40401, "user not found")
		return
	}

	utils.Success(ctx, sanitizeUserResponseWithAdmin(user))
}

// UpdateProfile allows the authenticated user to update basic profile fields.
func (a *AuthController) UpdateProfile(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		utils.Error(ctx, http.StatusUnauthorized, 40108, "unauthorized")
		return
	}

	var req struct {
		Email     string `json:"email"`
		Signature string `json:"signature"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Error(ctx, http.StatusBadRequest, 40030, "invalid request payload")
		return
	}

	var user models.User
	if err := a.db.First(&user, userID).Error; err != nil {
		utils.Error(ctx, http.StatusNotFound, 40401, "user not found")
		return
	}

	if strings.TrimSpace(req.Email) != "" {
		user.Email = strings.TrimSpace(req.Email)
	}
	if req.Signature != "" || (req.Signature == "" && strings.Contains(ctx.GetHeader("Content-Type"), "application/json")) {
		// Allow clearing signature when explicitly provided as empty string.
		sig := strings.TrimSpace(req.Signature)
		// Sanitize to avoid XSS; only keep safe subset if HTML is provided
		sig = utils.Sanitize(sig)
		// Limit length to 255 runes
		if len([]rune(sig)) > 255 {
			rs := []rune(sig)
			sig = string(rs[:255])
		}
		user.Signature = sig
	}

	if err := a.db.Save(&user).Error; err != nil {
		utils.Error(ctx, http.StatusInternalServerError, 50031, "failed to update profile")
		return
	}
	// Invalidate user public cache by id and username
	utils.InvalidateByPrefix("cache:user:public:" + strconv.Itoa(int(user.ID)))
	utils.InvalidateByPrefix("cache:user:public:uname:" + user.Username)

	utils.Success(ctx, sanitizeUserResponseWithAdmin(user))
}

func (a *AuthController) oauthConfig(provider string) (*oauth2.Config, error) {
	cfg := config.Get()
	switch strings.ToLower(provider) {
	case "github":
		if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" {
			return nil, fmt.Errorf("github oauth not configured")
		}
		return &oauth2.Config{
			ClientID:     cfg.GitHubClientID,
			ClientSecret: cfg.GitHubClientSecret,
			RedirectURL:  fmt.Sprintf("%s/api/v1/auth/oauth/github/callback", cfg.OAuthRedirectBase),
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
		}, nil
	case "google":
		if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
			return nil, fmt.Errorf("google oauth not configured")
		}
		return &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  fmt.Sprintf("%s/api/v1/auth/oauth/google/callback", cfg.OAuthRedirectBase),
			Scopes:       []string{"openid", "profile", "email"},
			Endpoint:     google.Endpoint,
		}, nil
	case "telegram":
		return nil, fmt.Errorf("telegram login uses dedicated endpoint")
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func (a *AuthController) fetchOAuthUser(provider string, token *oauth2.Token) (*oauthUser, error) {
	switch strings.ToLower(provider) {
	case "github":
		return fetchGitHubUser(token)
	case "google":
		return fetchGoogleUser(token)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

type oauthUser struct {
	ID          string
	Username    string
	DisplayName string
	Email       string
	AvatarURL   string
}

func (a *AuthController) findOrCreateOAuthUser(provider string, data *oauthUser) (*models.User, error) {
	var user models.User
	err := a.db.Where("provider = ? AND provider_id = ?", provider, data.ID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			email := strings.TrimSpace(data.Email)
			username := a.ensureUniqueUsername(data.Username, provider, data.ID)
			user = models.User{
				Username:   username,
				Email:      email,
				Provider:   provider,
				ProviderID: data.ID,
				AvatarURL:  data.AvatarURL,
				RegisterIP: "oauth",
			}

			if err := a.db.Create(&user).Error; err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		updates := map[string]interface{}{
			"email":      strings.TrimSpace(data.Email),
			"avatar_url": data.AvatarURL,
		}
		_ = a.db.Model(&user).Updates(updates)
	}

	return &user, nil
}

func fetchGitHubUser(token *oauth2.Token) (*oauthUser, error) {
	client := http.Client{}
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github user info request failed: %s", resp.Status)
	}

	var payload struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	email, _ := fetchGitHubEmail(token.AccessToken)

	return &oauthUser{
		ID:          fmt.Sprintf("%d", payload.ID),
		Username:    payload.Login,
		DisplayName: fallback(payload.Name, payload.Login),
		Email:       email,
		AvatarURL:   payload.AvatarURL,
	}, nil
}

func fetchGitHubEmail(accessToken string) (string, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github emails request failed: %s", resp.Status)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, email := range emails {
		if email.Primary && email.Verified {
			return email.Email, nil
		}
	}

	if len(emails) > 0 {
		return emails[0].Email, nil
	}

	return "", nil
}

func fetchGoogleUser(token *oauth2.Token) (*oauthUser, error) {
	req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google user info request failed: %s", resp.Status)
	}

	var payload struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return &oauthUser{
		ID:          payload.ID,
		Username:    payload.Email,
		DisplayName: payload.Name,
		Email:       payload.Email,
		AvatarURL:   payload.Picture,
	}, nil
}

func fallback(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func verifyTelegramSignature(botToken string, req telegramLoginRequest) bool {
	if botToken == "" {
		return false
	}

	values := map[string]string{
		"auth_date": fmt.Sprintf("%d", req.AuthDate),
		"id":        req.ID,
	}
	if req.Username != "" {
		values["username"] = req.Username
	}
	if req.FirstName != "" {
		values["first_name"] = req.FirstName
	}
	if req.LastName != "" {
		values["last_name"] = req.LastName
	}
	if req.PhotoURL != "" {
		values["photo_url"] = req.PhotoURL
	}

	pairs := make([]string, 0, len(values))
	for k, v := range values {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(pairs)
	dataCheckString := strings.Join(pairs, "\n")

	digest := sha256.Sum256([]byte(botToken))
	h := hmac.New(sha256.New, digest[:])
	h.Write([]byte(dataCheckString))
	expected := h.Sum(nil)
	provided, err := hex.DecodeString(strings.TrimSpace(req.Hash))
	if err != nil {
		return false
	}
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare(expected, provided) == 1
}

func sanitizeUsername(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			builder.WriteRune('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	return result
}

func (a *AuthController) ensureUniqueUsername(base, provider, id string) string {
	base = sanitizeUsername(base)
	if base == "" {
		base = sanitizeUsername(fmt.Sprintf("%s_%s", provider, id))
		if base == "" {
			base = fmt.Sprintf("user_%s", id)
		}
	}

	candidate := base
	suffix := 1
	for {
		var count int64
		if err := a.db.Model(&models.User{}).Where("username = ?", candidate).Count(&count).Error; err != nil {
			return candidate
		}
		if count == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s_%d", base, suffix)
		suffix++
	}
}

func sanitizeUserResponse(user models.User) gin.H {
	return gin.H{
		"id":               user.ID,
		"username":         user.Username,
		"email":            user.Email,
		"register_ip":      user.RegisterIP,
		"provider":         user.Provider,
		"avatar_url":       user.AvatarURL,
		"signature":        user.Signature,
		"points":           user.Points,
		"consecutive_days": user.ConsecutiveDays,
		"created_at":       user.CreatedAt,
	}
}

// isAdminUsername checks whether given username is configured as an admin (case-insensitive)
func isAdminUsername(username string) bool {
	uname := strings.TrimSpace(username)
	if uname == "" {
		return false
	}
	cfg := config.Get()
	for _, u := range cfg.AdminUsernames {
		if strings.EqualFold(strings.TrimSpace(u), uname) {
			return true
		}
	}
	return false
}

// sanitizeUserResponseWithAdmin includes is_admin for authenticated responses
func sanitizeUserResponseWithAdmin(user models.User) gin.H {
	m := sanitizeUserResponse(user)
	m["is_admin"] = isAdminUsername(user.Username)
	return m
}
