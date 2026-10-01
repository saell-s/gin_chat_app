package chat

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"gin/app/shared/middleware"
	"gin/app/shared/utils"
)

// Handler exposes chat endpoints.
type Handler struct {
	svc *Service
	hub Notifier
}

func NewHandler(svc *Service, hub Notifier) *Handler {
	return &Handler{svc: svc, hub: hub}
}

// List godoc
// @Summary      List conversations
// @Description  Returns the caller's conversations with peer, last message and unread count.
// @Tags         chat
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /api/v1/conversations [get]
func (h *Handler) List(c *gin.Context) {
	convs, err := h.svc.ListConversations(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, convs)
}

// Create godoc
// @Summary      Start a direct conversation
// @Description  Returns the existing direct conversation with the user or creates a new one.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  chat.CreateConversationInput  true  "Peer user id"
// @Success      201   {object}  map[string]any
// @Failure      400   {object}  map[string]any
// @Failure      404   {object}  map[string]any
// @Router       /api/v1/conversations [post]
func (h *Handler) Create(c *gin.Context) {
	var in CreateConversationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	view, err := h.svc.EnsureDirect(c.Request.Context(), middleware.UserID(c), in.UserID)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.Created(c, view)
}

// Messages godoc
// @Summary      Message history
// @Description  Returns messages of a conversation, oldest first. Supports cursor paging via before_id.
// @Tags         chat
// @Produce      json
// @Security     BearerAuth
// @Param        id         path   integer  true   "Conversation id"
// @Param        before_id  query  integer  false  "Return messages older than this id"
// @Param        limit      query  integer  false  "Page size (default 50, max 200)"
// @Success      200  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Router       /api/v1/conversations/{id}/messages [get]
func (h *Handler) Messages(c *gin.Context) {
	convID, ok := convParam(c)
	if !ok {
		return
	}
	beforeID := int64(0)
	if v := c.Query("before_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			beforeID = n
		}
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	msgs, err := h.svc.ListMessages(c.Request.Context(), middleware.UserID(c), convID, beforeID, limit)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, msgs)
}

// Send godoc
// @Summary      Send a message
// @Description  Stores a message and delivers it to participants over websocket.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path   integer              true  "Conversation id"
// @Param        body  body   chat.SendMessageInput  true  "Message body"
// @Success      201   {object}  map[string]any
// @Failure      403   {object}  map[string]any
// @Router       /api/v1/conversations/{id}/messages [post]
func (h *Handler) Send(c *gin.Context) {
	convID, ok := convParam(c)
	if !ok {
		return
	}
	var in SendMessageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	msg, err := h.svc.SendMessage(c.Request.Context(), middleware.UserID(c), convID, in.Body)
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.Created(c, msg)
}

// Read godoc
// @Summary      Mark conversation read
// @Description  Advances the read cursor and sends a read receipt to the other party.
// @Tags         chat
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path   integer          true  "Conversation id"
// @Param        body  body   chat.ReadInput   true  "Last read message id (0 for latest)"
// @Success      200   {object}  map[string]any
// @Failure      403   {object}  map[string]any
// @Router       /api/v1/conversations/{id}/read [post]
func (h *Handler) Read(c *gin.Context) {
	convID, ok := convParam(c)
	if !ok {
		return
	}
	var in ReadInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.Abort(c, utils.BadRequest(err.Error()))
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), middleware.UserID(c), convID, in.LastReadID); err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, gin.H{"read": true})
}

// Presence godoc
// @Summary      Online users
// @Description  Returns the ids of users with an active websocket connection.
// @Tags         chat
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]any
// @Router       /api/v1/presence [get]
func (h *Handler) Presence(c *gin.Context) {
	ids := []int64{}
	if h.hub != nil {
		ids = h.hub.OnlineIDs()
	}
	utils.OK(c, gin.H{"online_ids": ids})
}

// Directory godoc
// @Summary      User directory
// @Description  Lists active accounts the caller may start a chat with.
// @Tags         chat
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /api/v1/directory [get]
func (h *Handler) Directory(c *gin.Context) {
	users, err := h.svc.Directory(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		utils.Abort(c, err)
		return
	}
	utils.OK(c, users)
}

func convParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		utils.Abort(c, utils.BadRequest("invalid conversation id"))
		return 0, false
	}
	return id, true
}
