package reporting

import (
	"github.com/gin-gonic/gin"

	"gin/app/features/auth/rbac"
)

// RegisterRoutes wires the reporting endpoints onto the API group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, authorize gin.HandlerFunc) {
	g := rg.Group("/reports")
	{
		g.GET("/sales", authorize, rbac.Require(rbac.RoleEditor), h.Sales)
		g.GET("/sales.csv", authorize, rbac.Require(rbac.RoleEditor), h.ExportCSV)
		g.POST("/generate", authorize, rbac.Require(rbac.RoleEditor), h.Generate)
	}
}
