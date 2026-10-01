package chat

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the chat endpoints onto the API group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, authorize gin.HandlerFunc) {
	g := rg.Group("/conversations")
	{
		g.GET("", authorize, h.List)
		g.POST("", authorize, h.Create)
		g.GET("/:id/messages", authorize, h.Messages)
		g.POST("/:id/messages", authorize, h.Send)
		g.POST("/:id/read", authorize, h.Read)
	}
	rg.GET("/presence", authorize, h.Presence)
	rg.GET("/directory", authorize, h.Directory)
}
