package auth

import (
	"github.com/gin-gonic/gin"

	"gin/app/features/user"
	"gin/app/shared/middleware"
	"gin/app/shared/utils"
)

// Handler exposes authentication endpoints.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register godoc
// @Summary      Register
// @Description  Creates a new account and returns its first token pair.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  user.RegisterInput  true  "Registration payload"
// @Success      201   {object}  map[string]any
// @Failure      400   {object}  map[string]any
// @Failure      409   {object}  map[string]any
// @Router       /api/v1/auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var in user.RegisterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	pair, err := h.svc.Register(c.Request.Context(), in)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.Created(c, pair)
}

// Login godoc
// @Summary      Login
// @Description  Exchanges email and password for a token pair.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  auth.LoginInput  true  "Credentials"
// @Success      200   {object}  map[string]any
// @Failure      401   {object}  map[string]any
// @Failure      403   {object}  map[string]any
// @Router       /api/v1/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var in LoginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	pair, err := h.svc.Login(c.Request.Context(), in)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, pair)
}

// Refresh godoc
// @Summary      Refresh tokens
// @Description  Rotates the refresh token and returns a new token pair.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  auth.RefreshInput  true  "Refresh token"
// @Success      200   {object}  map[string]any
// @Failure      401   {object}  map[string]any
// @Router       /api/v1/auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var in RefreshInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), in.RefreshToken)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, pair)
}

// Logout godoc
// @Summary      Logout
// @Description  Revokes the presented refresh token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  auth.RefreshInput  true  "Refresh token"
// @Success      200   {object}  map[string]any
// @Failure      401   {object}  map[string]any
// @Router       /api/v1/auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	var in RefreshInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	if err := h.svc.Logout(c.Request.Context(), in.RefreshToken); err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, gin.H{"logged_out": true})
}

// Me godoc
// @Summary      Current identity
// @Description  Returns the account tied to the presented access token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /api/v1/auth/me [get]
func (h *Handler) Me(c *gin.Context) {
	u, err := h.svc.Me(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, u)
}
