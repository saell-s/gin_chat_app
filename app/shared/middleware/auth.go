package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gin/app/shared/security"
	"gin/app/shared/utils"
)

const (
	// ContextClaims is the gin context key holding the verified JWT claims.
	ContextClaims = "auth.claims"
	bearerPrefix  = "Bearer "
)

// Authorize validates the Bearer access token and stores the claims in the context.
func Authorize(tm *security.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			utils.Abort(c, utils.Unauthorized("missing authorization header"))
			return
		}
		if !strings.HasPrefix(header, bearerPrefix) {
			utils.Abort(c, utils.Unauthorized("token must be a bearer token"))
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
		claims, err := tm.ParseAccess(raw)
		if err != nil {
			if errors.Is(err, security.ErrTokenExpired) {
				utils.Abort(c, utils.Unauthorized("token expired"))
				return
			}
			utils.Abort(c, utils.Unauthorized("invalid token"))
			return
		}
		c.Set(ContextClaims, claims)
		c.Next()
	}
}

// Claims returns the verified claims stored by Authorize.
func Claims(c *gin.Context) (*security.Claims, bool) {
	v, ok := c.Get(ContextClaims)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*security.Claims)
	return claims, ok
}

// UserID returns the authenticated user id or 0.
func UserID(c *gin.Context) int64 {
	claims, ok := Claims(c)
	if !ok {
		return 0
	}
	return UserIDFromClaims(claims)
}

// UserIDFromClaims extracts the numeric subject from verified claims.
func UserIDFromClaims(claims *security.Claims) int64 {
	if claims == nil {
		return 0
	}
	var id int64
	for _, r := range claims.Subject {
		if r < '0' || r > '9' {
			return 0
		}
		id = id*10 + int64(r-'0')
	}
	return id
}

// RequireAuth aborts when no authenticated user is present.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := Claims(c); !ok {
			utils.Abort(c, utils.Unauthorized("authentication required"))
			return
		}
		c.Next()
	}
}

// NotFound registers a JSON 404 fallback for unknown routes.
func NotFound() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{"message": "route not found", "status": http.StatusNotFound},
		})
	}
}
