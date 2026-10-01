package user

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"gin/app/shared/middleware"
	"gin/app/shared/utils"
)

// Handler exposes user endpoints.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ListUsers godoc
// @Summary      List users
// @Description  Returns a paginated list of users, filterable by role, status and free text.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page        query integer false "Page number (default 1)"
// @Param        page_size   query integer false "Page size (default 20, max 100)"
// @Param        role        query string  false "Filter by role (admin|editor|user)"
// @Param        status      query string  false "Filter by status (active|suspended)"
// @Param        q           query string  false "Search in email or name"
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Router       /api/v1/users [get]
func (h *Handler) List(c *gin.Context) {
	page := utils.ParsePage(c)
	users, total, err := h.svc.List(c.Request.Context(), ListFilter{
		Role:     c.Query("role"),
		Status:   c.Query("status"),
		Query:    c.Query("q"),
		Page:     page.Page,
		PageSize: page.PageSize,
	})
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.Paginated(c, users, page.Page, page.PageSize, total)
}

// Get godoc
// @Summary      Get a user
// @Description  Returns a single user by id. Admins may read anyone, others only themselves.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path   integer  true  "User id"
// @Success      200  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /api/v1/users/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if !h.canRead(c, id) {
		utils.Abort(c, utils.Forbidden("you may only read your own profile"))
		return
	}
	u, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, u)
}

// GetMe godoc
// @Summary      Get current user
// @Description  Returns the profile of the authenticated user.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /api/v1/users/me [get]
func (h *Handler) GetMe(c *gin.Context) {
	u, err := h.svc.GetByID(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, u)
}

// UpdateMe godoc
// @Summary      Update current user
// @Description  Updates the name and/or email of the authenticated user.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  user.UpdateProfileInput  true  "Profile patch"
// @Success      200   {object}  map[string]any
// @Failure      400   {object}  map[string]any
// @Failure      409   {object}  map[string]any
// @Router       /api/v1/users/me [patch]
func (h *Handler) UpdateMe(c *gin.Context) {
	var in UpdateProfileInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	u, err := h.svc.UpdateProfile(c.Request.Context(), middleware.UserID(c), in)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, u)
}

// Update godoc
// @Summary      Update a user
// @Description  Admin only: updates name, role and status of any user.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path   integer              true  "User id"
// @Param        body  body   user.AdminUpdateInput  true  "User patch"
// @Success      200   {object}  map[string]any
// @Failure      400   {object}  map[string]any
// @Failure      403   {object}  map[string]any
// @Router       /api/v1/users/{id} [patch]
func (h *Handler) Update(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in AdminUpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	u, err := h.svc.AdminUpdate(c.Request.Context(), id, in)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, u)
}

// Delete godoc
// @Summary      Delete a user
// @Description  Admin only: removes a user and revokes their sessions.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path   integer  true  "User id"
// @Success      200  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /api/v1/users/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, gin.H{"deleted": id})
}

// ChangePassword godoc
// @Summary      Change password
// @Description  Rotates the password of the authenticated user.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  user.ChangePasswordInput  true  "Password change"
// @Success      200   {object}  map[string]any
// @Failure      400   {object}  map[string]any
// @Router       /api/v1/users/me/password [patch]
func (h *Handler) ChangePassword(c *gin.Context) {
	var in ChangePasswordInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), middleware.UserID(c), in); err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, gin.H{"changed": true})
}

func (h *Handler) canRead(c *gin.Context, targetID int64) bool {
	claims, ok := middleware.Claims(c)
	if !ok {
		return false
	}
	return CanManage(middleware.UserID(c), claims.Role, targetID)
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		utils.Abort(c, utils.BadRequest("invalid user id"))
		return 0, false
	}
	return id, true
}
