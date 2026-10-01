package call

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the call endpoints onto the API group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, authorize gin.HandlerFunc) {
	g := rg.Group("/calls")
	{
		g.GET("", authorize, h.History)
		g.GET("/:id", authorize, h.Get)
	}
}
