package web

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the server rendered screens.
func RegisterRoutes(r *gin.Engine) {
	r.GET("/", page(LoginView()))
	r.GET("/login", page(LoginView()))
	r.GET("/app", page(ChatView()))
}

func page(component templ.Component) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		if err := component.Render(c.Request.Context(), c.Writer); err != nil {
			c.Error(err) //nolint:errcheck
		}
	}
}
