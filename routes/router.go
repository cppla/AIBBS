package routes

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/cppla/aibbs/config"
	"github.com/cppla/aibbs/controllers"
	"github.com/cppla/aibbs/middleware"
	"github.com/cppla/aibbs/utils"
)

// SetupRouter wires routes, middlewares, and controllers.
func SetupRouter(db *gorm.DB) *gin.Engine {
	// Load config and set Gin mode from configuration
	cfg := config.Get()
	switch strings.ToLower(cfg.GinMode) {
	case "debug":
		gin.SetMode(gin.DebugMode)
	case "test":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Replace default console logger with file-based zap logger
	ginLogPath := cfg.GinPath
	// Use application log level as reference
	gl, err := utils.NewRollingFileLogger(ginLogPath, cfg.LogLevel, cfg.LogMaxSizeMB, cfg.LogMaxBackups, cfg.LogMaxAgeDays, cfg.LogCompress)
	if err == nil {
		r.Use(utils.Ginzap(gl, time.RFC3339, true))
		r.Use(utils.RecoveryWithZap(gl, false))
	} else {
		// fallback to default recovery if logger failed to init
		r.Use(gin.Recovery())
	}

	corsCfg := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	if len(cfg.AllowedOrigins) == 1 && cfg.AllowedOrigins[0] == "*" {
		corsCfg.AllowAllOrigins = true
	} else {
		corsCfg.AllowOrigins = cfg.AllowedOrigins
	}

	r.Use(cors.New(corsCfg))
	// Country allow/deny filter (deny has priority)
	r.Use(middleware.CountryFilter())
	// Record PV after each request
	r.Use(middleware.PageViewRecorder(db))

	r.Static("/static", "./static")

	r.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})

	r.GET("/register", func(c *gin.Context) {
		c.File("./static/register.html")
	})

	r.GET("/api", func(c *gin.Context) {
		c.File("./static/api.html")
	})

	r.GET("/personal/:username", func(c *gin.Context) {
		c.File("./static/personal.html")
	})

	r.GET("/health", func(ctx *gin.Context) {
		utils.Success(ctx, gin.H{"status": "ok"})
	})

	authController := controllers.NewAuthController(db)
	postController := controllers.NewPostController(db)
	signController := controllers.NewSignInController(db)
	statsController := controllers.NewStatsController(db)
	configController := controllers.NewConfigController()

	api := r.Group("/api/v1")

	authGroup := api.Group("/auth")
	authGroup.Use(middleware.RateLimitMiddleware())
	authGroup.POST("/register", authController.Register)
	authGroup.POST("/login", authController.Login)
	authGroup.POST("/send-email-code", authController.SendEmailCode)
	authGroup.GET("/captcha", authController.Captcha)
	authGroup.POST("/captcha/verify", authController.CaptchaVerify)
	authGroup.POST("/telegram", authController.TelegramLogin)
	authGroup.GET("/oauth/:provider/login", authController.OAuthRedirect)
	authGroup.GET("/oauth/:provider/callback", authController.OAuthCallback)
	authGroup.POST("/logout", middleware.AuthRequired(), authController.Logout)
	authGroup.GET("/me", middleware.AuthRequired(), authController.Me)
	authGroup.PATCH("/profile", middleware.AuthRequired(), authController.UpdateProfile)

	postsGroup := api.Group("/posts")
	postsGroup.GET("", postController.ListPosts)
	postsGroup.GET("/:id", postController.GetPost)

	// Public stats endpoint
	api.GET("/stats", statsController.GetStats)
	api.GET("/posts/:id/stats", statsController.GetPostStats)
	// Public config endpoint
	api.GET("/config/footer", configController.GetFooter)
	api.GET("/config/notice", configController.GetNotice)
	// Public user posts
	api.GET("/users/:id/posts", postController.ListUserPosts)

	// Public user by username
	api.GET("/user/by-username/:username", authController.GetUserPublicByUsername)

	protected := api.Group("")
	protected.Use(middleware.AuthRequired(), middleware.RateLimitMiddleware())
	// Public user profile
	api.GET("/users/:id", authController.GetUserPublic)

	protected.GET("/users", authController.ListUsers)
	protected.POST("/upload", postController.UploadAttachment)
	protected.POST("/posts", postController.CreatePost)
	protected.PUT("/posts/:id", postController.UpdatePost)
	protected.DELETE("/posts/:id", postController.DeletePost)
	protected.POST("/posts/:id/comments", postController.CreateComment)
	protected.DELETE("/comments/:commentId", postController.DeleteComment)
	protected.GET("/users/me/posts", postController.ListMyPosts)
	protected.POST("/signin/daily", signController.DailySignIn)
	protected.GET("/signin/status", signController.SignInStatus)

	r.NoRoute(func(ctx *gin.Context) {
		path := ctx.Request.URL.Path
		// API 未命中：返回 API 404 JSON
		if strings.HasPrefix(path, "/api/") {
			utils.Error(ctx, http.StatusNotFound, 40400, "api route not found")
			return
		}
		// 静态资源未命中：仍按 404 处理
		if strings.HasPrefix(path, "/static/") {
			ctx.JSON(http.StatusNotFound, gin.H{"message": "static asset not found"})
			return
		}
		// 其余路径（如 /post-4-1, /categories/tech）回退到 SPA 入口
		ctx.Status(http.StatusOK)
		ctx.File("./static/index.html")
	})

	return r
}
