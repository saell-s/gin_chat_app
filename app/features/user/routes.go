package user

import (
	"github.com/gin-gonic/gin"

	"gin/app/features/auth/rbac"
)

// RegisterRoutes wires the user endpoints onto the API group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, authorize gin.HandlerFunc) {
	g := rg.Group("/users")
	{
		g.GET("/me", authorize, h.GetMe)
		g.PATCH("/me", authorize, h.UpdateMe)
		g.PATCH("/me/password", authorize, h.ChangePassword)

		g.GET("", authorize, rbac.Require(rbac.RoleAdmin), h.List)
		g.GET("/:id", authorize, h.Get)
		g.PATCH("/:id", authorize, rbac.Require(rbac.RoleAdmin), h.Update)
		g.DELETE("/:id", authorize, rbac.Require(rbac.RoleAdmin), h.Delete)
	}
}
