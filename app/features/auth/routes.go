package auth

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the authentication endpoints onto the API group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, authorize gin.HandlerFunc) {
	g := rg.Group("/auth")
	{
		g.POST("/register", h.Register)
		g.POST("/login", h.Login)
		g.POST("/refresh", h.Refresh)
		g.POST("/logout", authorize, h.Logout)
		g.GET("/me", authorize, h.Me)
	}
}
