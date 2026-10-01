package reporting

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gin/app/shared/middleware"
	"gin/app/shared/utils"
)

// Handler exposes reporting endpoints.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Sales godoc
// @Summary      Sales report
// @Description  Aggregated sales time series grouped by day, week or month.
// @Tags         reporting
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        from       query  string  false  "Range start (RFC3339 or YYYY-MM-DD, default -30 days)"
// @Param        to         query  string  false  "Range end (RFC3339 or YYYY-MM-DD, default now)"
// @Param        group_by   query  string  false  "day|week|month (default day)"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /api/v1/reports/sales [get]
func (h *Handler) Sales(c *gin.Context) {
	from, to, err := parseRange(c)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	report, err := h.svc.SalesReport(c.Request.Context(), from, to, c.Query("group_by"))
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, report)
}

// ExportCSV godoc
// @Summary      Export orders as CSV
// @Description  Streams the raw orders of the requested range as a CSV file.
// @Tags         reporting
// @Accept       json
// @Produce      text/csv
// @Security     BearerAuth
// @Param        from  query  string  false  "Range start (RFC3339 or YYYY-MM-DD)"
// @Param        to    query  string  false  "Range end (RFC3339 or YYYY-MM-DD)"
// @Success      200  {string}  string  "text/csv"
// @Failure      400  {object}  map[string]any
// @Router       /api/v1/reports/sales.csv [get]
func (h *Handler) ExportCSV(c *gin.Context) {
	from, to, err := parseRange(c)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	body, filename, err := h.svc.CSV(c.Request.Context(), from, to)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", body)
}

// Generate godoc
// @Summary      Generate a report
// @Description  Builds a sales report and records a report.generated event.
// @Tags         reporting
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        from      query  string  false  "Range start (RFC3339 or YYYY-MM-DD)"
// @Param        to        query  string  false  "Range end (RFC3339 or YYYY-MM-DD)"
// @Param        group_by  query  string  false  "day|week|month (default day)"
// @Success      201  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /api/v1/reports/generate [post]
func (h *Handler) Generate(c *gin.Context) {
	from, to, err := parseRange(c)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	actor := ""
	if claims, ok := middleware.Claims(c); ok {
		actor = claims.Email
	}
	report, err := h.svc.Generate(c.Request.Context(), actor, from, to, c.Query("group_by"))
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.Created(c, report)
}

func parseRange(c *gin.Context) (time.Time, time.Time, error) {
	from, err := ParseDate(c.Query("from"))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := ParseDate(c.Query("to"))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to, nil
}
