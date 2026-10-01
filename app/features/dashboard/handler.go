package dashboard

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"gin/app/shared/utils"
)

// Handler exposes dashboard endpoints.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Summary godoc
// @Summary      Dashboard summary
// @Description  Returns aggregated user, order and product statistics.
// @Tags         dashboard
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Router       /api/v1/dashboard/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	summary, err := h.svc.Summary(c.Request.Context())
	if err != nil {
		utils.Abort(c, utils.Internal(err))
		return
	}
	utils.OK(c, summary)
}

// Activity godoc
// @Summary      Recent activity
// @Description  Returns the latest recorded domain events.
// @Tags         dashboard
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query  integer  false  "Number of events (default 20, max 100)"
// @Success      200    {object}  map[string]any
// @Failure      401    {object}  map[string]any
// @Router       /api/v1/dashboard/activity [get]
func (h *Handler) Activity(c *gin.Context) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	page, err := h.svc.Activity(c.Request.Context(), limit)
	if err != nil {
		utils.Abort(c, utils.Internal(err))
		return
	}
	utils.OK(c, page)
}
