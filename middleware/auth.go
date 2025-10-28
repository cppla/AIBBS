package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cppla/aibbs/utils"
)

const (
	// ContextUserIDKey is the key used to store authenticated user ID in Gin context.
	ContextUserIDKey = "user_id"
	// ContextUsernameKey stores the username inside Gin context.
	ContextUsernameKey = "username"
)

// AuthRequired ensures the request is authenticated via JWT.
func AuthRequired() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			utils.Error(ctx, http.StatusUnauthorized, 40101, "authorization header missing")
			ctx.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			utils.Error(ctx, http.StatusUnauthorized, 40102, "invalid authorization header format")
			ctx.Abort()
			return
		}

		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			utils.Error(ctx, http.StatusUnauthorized, 40103, "empty bearer token")
			ctx.Abort()
			return
		}

		if utils.IsTokenBlacklisted(tokenString) {
			utils.Error(ctx, http.StatusUnauthorized, 40104, "token revoked")
			ctx.Abort()
			return
		}

		claims, err := utils.ParseToken(tokenString)
		if err != nil {
			utils.Error(ctx, http.StatusUnauthorized, 40105, "invalid token")
			ctx.Abort()
			return
		}

		ctx.Set(ContextUserIDKey, claims.UserID)
		ctx.Set(ContextUsernameKey, claims.Username)
		ctx.Next()
	}
}
