package chat

import (
	"context"
	"errors"
	"sort"
	"time"

	"gin/app/features/user"
	"gin/app/shared/realtime"
	"gin/app/shared/utils"
)

// Notifier is the slice of the realtime hub the chat service needs.
type Notifier interface {
	SendToUser(userID int64, ev realtime.Event) bool
	SendToUsers(userIDs []int64, ev realtime.Event) map[int64]bool
	Online(userID int64) bool
	OnlineIDs() []int64
}

// Service implements chat use cases and realtime fan-out.
type Service struct {
	repo  Repository
	users user.Repository
	hub   Notifier
}

func NewService(repo Repository, users user.Repository, hub Notifier) *Service {
	return &Service{repo: repo, users: users, hub: hub}
}

// ListConversations returns the viewer's conversations, newest activity first.
func (s *Service) ListConversations(ctx context.Context, viewerID int64) ([]ConversationView, error) {
	recs, err := s.repo.ListConversations(ctx, viewerID)
	if err != nil {
		return nil, utils.Internal(err)
	}
	out := make([]ConversationView, 0, len(recs))
	for _, rec := range recs {
		view, err := s.view(ctx, rec, viewerID)
		if err != nil {
			return nil, err
		}
		out = append(out, *view)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// EnsureDirect returns the direct conversation with peerID, creating it if needed.
func (s *Service) EnsureDirect(ctx context.Context, viewerID, peerID int64) (*ConversationView, error) {
	if viewerID == peerID {
		return nil, utils.BadRequest("you cannot start a conversation with yourself")
	}
	peer, err := s.users.GetByID(ctx, peerID)
	if err != nil {
		return nil, utils.NotFound("user not found")
	}
	if peer.Status != user.StatusActive {
		return nil, utils.BadRequest("user is not active")
	}

	rec, err := s.repo.FindDirect(ctx, viewerID, peerID)
	if err != nil {
		if !errors.Is(err, utils.ErrNotFound) {
			return nil, utils.Internal(err)
		}
		rec, err = s.repo.CreateConversation(ctx, TypeDirect, []int64{viewerID, peerID})
		if err != nil {
			return nil, utils.Internal(err)
		}
		rec.PeerID = peerID
	}
	return s.view(ctx, *rec, viewerID)
}

// ListMessages returns message history, oldest first.
func (s *Service) ListMessages(ctx context.Context, viewerID, conversationID, beforeID int64, limit int) ([]Message, error) {
	if err := s.requireParticipant(ctx, conversationID, viewerID); err != nil {
		return nil, err
	}
	msgs, err := s.repo.ListMessages(ctx, conversationID, beforeID, limit)
	if err != nil {
		return nil, utils.Internal(err)
	}
	return msgs, nil
}

// SendMessage stores a message and pushes it to every participant.
func (s *Service) SendMessage(ctx context.Context, viewerID, conversationID int64, body string) (*Message, error) {
	if err := s.requireParticipant(ctx, conversationID, viewerID); err != nil {
		return nil, err
	}
	msg := &Message{
		ConversationID: conversationID,
		SenderID:       viewerID,
		Kind:           KindText,
		Body:           body,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.repo.AddMessage(ctx, msg); err != nil {
		return nil, utils.Internal(err)
	}
	if err := s.repo.TouchConversation(ctx, conversationID, msg.CreatedAt); err != nil {
		return nil, utils.Internal(err)
	}
	// The sender has implicitly read their own message.
	_ = s.repo.MarkRead(ctx, conversationID, viewerID, msg.ID)

	if s.hub != nil {
		s.hub.SendToUser(viewerID, realtime.NewEvent("message.created", map[string]any{
			"conversation_id": conversationID,
			"message":         msg,
		}))
		if peers, err := s.repo.ParticipantIDs(ctx, conversationID); err == nil {
			for _, peerID := range peers {
				if peerID != viewerID {
					s.hub.SendToUser(peerID, realtime.NewEvent("message.created", map[string]any{
						"conversation_id": conversationID,
						"message":         msg,
					}))
				}
			}
		}
	}
	return msg, nil
}

// MarkRead advances the read cursor and informs the other party.
func (s *Service) MarkRead(ctx context.Context, viewerID, conversationID, lastReadID int64) error {
	if err := s.requireParticipant(ctx, conversationID, viewerID); err != nil {
		return err
	}
	if lastReadID <= 0 {
		if last, ok, err := s.repo.LastMessage(ctx, conversationID); err == nil && ok {
			lastReadID = last.ID
		} else {
			return nil
		}
	}
	if err := s.repo.MarkRead(ctx, conversationID, viewerID, lastReadID); err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return utils.NotFound("conversation not found")
		}
		return utils.Internal(err)
	}
	if s.hub != nil {
		if peers, err := s.repo.ParticipantIDs(ctx, conversationID); err == nil {
			for _, peerID := range peers {
				if peerID != viewerID {
					s.hub.SendToUser(peerID, realtime.NewEvent("read.receipt", map[string]any{
						"conversation_id": conversationID,
						"user_id":         viewerID,
						"last_read_id":    lastReadID,
					}))
				}
			}
		}
	}
	return nil
}

// NotifyTyping relays a typing indicator to the other participants.
func (s *Service) NotifyTyping(ctx context.Context, viewerID, conversationID int64, isTyping bool) error {
	if err := s.requireParticipant(ctx, conversationID, viewerID); err != nil {
		return err
	}
	if s.hub == nil {
		return nil
	}
	peers, err := s.repo.ParticipantIDs(ctx, conversationID)
	if err != nil {
		return utils.Internal(err)
	}
	for _, peerID := range peers {
		if peerID != viewerID {
			s.hub.SendToUser(peerID, realtime.NewEvent("typing", map[string]any{
				"conversation_id": conversationID,
				"user_id":         viewerID,
				"is_typing":       isTyping,
			}))
		}
	}
	return nil
}

// Directory lists active accounts that can be messaged (never includes the caller).
func (s *Service) Directory(ctx context.Context, viewerID int64) ([]user.User, error) {
	users, _, err := s.users.List(ctx, user.ListFilter{
		Status:   user.StatusActive,
		Page:     1,
		PageSize: 200,
	})
	if err != nil {
		return nil, err
	}
	out := make([]user.User, 0, len(users))
	for _, u := range users {
		if u.ID != viewerID {
			out = append(out, u)
		}
	}
	return out, nil
}

// IsParticipant reports conversation membership.
func (s *Service) IsParticipant(ctx context.Context, conversationID, userID int64) (bool, error) {
	return s.repo.IsParticipant(ctx, conversationID, userID)
}

// ParticipantIDs lists every member of a conversation.
func (s *Service) ParticipantIDs(ctx context.Context, conversationID int64) ([]int64, error) {
	return s.repo.ParticipantIDs(ctx, conversationID)
}

// HandleClientEvent dispatches inbound websocket frames owned by chat.
func (s *Service) HandleClientEvent(ctx context.Context, userID int64, typ string, data map[string]any) error {
	switch typ {
	case "typing.start", "typing.stop":
		return s.NotifyTyping(ctx, userID, int64(num(data, "conversation_id")), typ == "typing.start")
	default:
		return utils.BadRequest("unknown event type: " + typ)
	}
}

func num(data map[string]any, key string) float64 {
	if v, ok := data[key].(float64); ok {
		return v
	}
	return 0
}

func (s *Service) requireParticipant(ctx context.Context, conversationID, userID int64) error {
	if conversationID <= 0 {
		return utils.BadRequest("invalid conversation id")
	}
	ok, err := s.repo.IsParticipant(ctx, conversationID, userID)
	if err != nil {
		return utils.Internal(err)
	}
	if !ok {
		return utils.Forbidden("you are not part of this conversation")
	}
	return nil
}

func (s *Service) view(ctx context.Context, rec ConversationRecord, viewerID int64) (*ConversationView, error) {
	view := &ConversationView{
		ID:              rec.ID,
		Type:            rec.Type,
		LastReadMessage: rec.LastReadMessageID,
		CreatedAt:       rec.CreatedAt,
		UpdatedAt:       rec.CreatedAt,
	}
	if rec.PeerID > 0 {
		if peer, err := s.users.GetByID(ctx, rec.PeerID); err == nil {
			view.Peer = peer
		}
	}
	if last, ok, err := s.repo.LastMessage(ctx, rec.ID); err == nil && ok {
		view.LastMessage = last
		view.UpdatedAt = last.CreatedAt
	}
	if rec.LastMessageAt != nil && rec.LastMessageAt.After(view.UpdatedAt) {
		view.UpdatedAt = *rec.LastMessageAt
	}
	unread, err := s.repo.UnreadCount(ctx, rec.ID, viewerID, rec.LastReadMessageID)
	if err != nil {
		return nil, utils.Internal(err)
	}
	view.Unread = unread
	return view, nil
}
