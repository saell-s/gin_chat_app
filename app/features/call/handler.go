package call

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"gin/app/shared/middleware"
	"gin/app/shared/utils"
)

// Handler exposes call history endpoints. Live call control happens on /ws.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// History godoc
// @Summary      Call history
// @Description  Returns the caller's recent calls with direction, status and duration.
// @Tags         calls
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query  integer  false  "Number of calls (default 30, max 100)"
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /api/v1/calls [get]
func (h *Handler) History(c *gin.Context) {
	limit := 30
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	calls, err := h.svc.History(c.Request.Context(), middleware.UserID(c), limit)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, calls)
}

// Get godoc
// @Summary      Get one call
// @Description  Returns a single call record visible to the caller.
// @Tags         calls
// @Produce      json
// @Security     BearerAuth
// @Param        id   path   integer  true  "Call id"
// @Success      200  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /api/v1/calls/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		utils.Abort(c, utils.BadRequest("invalid call id"))
		return
	}
	calls, err := h.svc.History(c.Request.Context(), middleware.UserID(c), 100)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	for i := range calls {
		if calls[i].ID == id {
			utils.OK(c, calls[i])
			return
		}
	}
	utils.Abort(c, utils.NotFound("call not found"))
}
