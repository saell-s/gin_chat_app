package dashboard

import (
	"github.com/gin-gonic/gin"

	"gin/app/features/auth/rbac"
)

// RegisterRoutes wires the dashboard endpoints onto the API group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, authorize gin.HandlerFunc) {
	g := rg.Group("/dashboard")
	{
		g.GET("/summary", authorize, rbac.Require(rbac.RoleEditor), h.Summary)
		g.GET("/activity", authorize, rbac.Require(rbac.RoleEditor), h.Activity)
	}
}
