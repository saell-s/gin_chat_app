package call

import (
	"context"
	"sync"
	"time"

	"gin/app/features/chat"
	"gin/app/features/user"
	"gin/app/shared/db"
	"gin/app/shared/kafka"
	"gin/app/shared/realtime"
	"gin/app/shared/utils"
)

// Notifier is the slice of the realtime hub the call service needs.
type Notifier interface {
	SendToUser(userID int64, ev realtime.Event) bool
	Online(userID int64) bool
}

// Service owns the call state machine and WebRTC signaling relay.
type Service struct {
	repo   Repository
	chat   *chat.Service
	users  user.Repository
	hub    Notifier
	events db.EventRepository
	pub    kafka.Publisher

	mu sync.Mutex
}

func NewService(repo Repository, chatSvc *chat.Service, users user.Repository, hub Notifier, events db.EventRepository, pub kafka.Publisher) *Service {
	return &Service{repo: repo, chat: chatSvc, users: users, hub: hub, events: events, pub: pub}
}

// Invite creates a ringing call and notifies the callee.
func (s *Service) Invite(ctx context.Context, callerID, calleeID int64, kind string) (*Call, error) {
	if kind != KindAudio && kind != KindVideo {
		return nil, utils.BadRequest("kind must be audio or video")
	}
	if callerID == calleeID {
		return nil, utils.BadRequest("you cannot call yourself")
	}
	peer, err := s.users.GetByID(ctx, calleeID)
	if err != nil {
		return nil, utils.NotFound("user not found")
	}
	if peer.Status != user.StatusActive {
		return nil, utils.BadRequest("user is not available")
	}

	view, err := s.chat.EnsureDirect(ctx, callerID, calleeID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	c := &Call{
		CallerID:       callerID,
		CalleeID:       calleeID,
		ConversationID: view.ID,
		Kind:           kind,
		Status:         StatusRinging,
		StartedAt:      now,
		CreatedAt:      now,
	}

	s.mu.Lock()
	if err := s.repo.Create(ctx, c); err != nil {
		s.mu.Unlock()
		return nil, utils.Internal(err)
	}
	s.mu.Unlock()

	s.emit(c, "call.ringing", callerID, nil)
	s.emit(c, "call.incoming", calleeID, nil)
	s.record(ctx, "call.initiated", c)
	go s.watchRing(c.ID)
	return s.decorate(c, callerID), nil
}

// Accept flips a ringing call to active and tells the caller to start WebRTC.
func (s *Service) Accept(ctx context.Context, userID, callID int64) (*Call, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.repo.Get(ctx, callID)
	if err != nil {
		return nil, utils.NotFound("call not found")
	}
	if c.CalleeID != userID {
		return nil, utils.Forbidden("only the callee can accept")
	}
	if c.Status != StatusRinging {
		return nil, utils.Conflict("call is no longer ringing")
	}
	now := time.Now().UTC()
	c.Status = StatusActive
	c.AnsweredAt = &now
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, utils.Internal(err)
	}
	s.emit(c, "call.accepted", c.CallerID, nil)
	s.emit(c, "call.active", userID, nil)
	s.record(ctx, "call.answered", c)
	return s.decorate(c, userID), nil
}

// Decline rejects a ringing call.
func (s *Service) Decline(ctx context.Context, userID, callID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.repo.Get(ctx, callID)
	if err != nil {
		return utils.NotFound("call not found")
	}
	if c.CalleeID != userID {
		return utils.Forbidden("only the callee can decline")
	}
	if c.Status != StatusRinging {
		return utils.Conflict("call is no longer ringing")
	}
	now := time.Now().UTC()
	c.Status = StatusDeclined
	c.EndedAt = &now
	if err := s.repo.Update(ctx, c); err != nil {
		return utils.Internal(err)
	}
	s.emit(c, "call.declined", c.CallerID, nil)
	s.record(ctx, "call.declined", c)
	return nil
}

// End terminates a ringing or active call.
func (s *Service) End(ctx context.Context, userID, callID int64, reason string) (*Call, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.repo.Get(ctx, callID)
	if err != nil {
		return nil, utils.NotFound("call not found")
	}
	if !c.IsParticipant(userID) {
		return nil, utils.Forbidden("you are not part of this call")
	}
	switch c.Status {
	case StatusRinging:
		if userID == c.CallerID {
			c.Status = StatusCancelled
		} else {
			c.Status = StatusDeclined
		}
	case StatusActive:
		c.Status = StatusEnded
		if c.AnsweredAt != nil {
			c.DurationSeconds = int64(time.Since(*c.AnsweredAt).Seconds())
		}
	default:
		return nil, utils.Conflict("call already finished")
	}
	now := time.Now().UTC()
	c.EndedAt = &now
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, utils.Internal(err)
	}
	peer := c.Peer(userID)
	s.emit(c, "call.ended", peer, map[string]any{"by": userID, "reason": reason})
	s.emit(c, "call.ended", userID, map[string]any{"by": userID, "reason": reason})
	s.record(ctx, "call.ended", c)
	return s.decorate(c, userID), nil
}

// Relay forwards WebRTC signaling to the other party.
func (s *Service) Relay(ctx context.Context, userID, callID int64, typ string, data map[string]any) error {
	c, err := s.repo.Get(ctx, callID)
	if err != nil {
		return utils.NotFound("call not found")
	}
	if !c.IsParticipant(userID) {
		return utils.Forbidden("you are not part of this call")
	}
	if c.Status != StatusActive {
		return utils.Conflict("call is not active")
	}
	if c.Kind == KindAudio && typ == "call.video-offer" {
		return utils.BadRequest("call is audio only")
	}

	payload := map[string]any{
		"call_id": c.ID,
		"from":    userID,
		"kind":    c.Kind,
	}
	for k, v := range data {
		payload[k] = v
	}
	peer := c.Peer(userID)
	if !s.hub.SendToUser(peer, realtime.NewEvent(typ, payload)) && typ != "call.ice" {
		return utils.BadRequest("peer is offline")
	}
	return nil
}

// History returns the user's recent calls, newest first.
func (s *Service) History(ctx context.Context, userID int64, limit int) ([]Call, error) {
	calls, err := s.repo.ListForUser(ctx, userID, limit)
	if err != nil {
		return nil, utils.Internal(err)
	}
	out := make([]Call, 0, len(calls))
	for i := range calls {
		out = append(out, *s.decorate(&calls[i], userID))
	}
	return out, nil
}

// HandleClientEvent dispatches inbound websocket frames owned by calls.
func (s *Service) HandleClientEvent(ctx context.Context, userID int64, typ string, data map[string]any) error {
	switch typ {
	case "call.invite":
		_, err := s.Invite(ctx, userID, int64(num(data, "callee_id")), str(data, "kind", KindAudio))
		return err
	case "call.accept":
		_, err := s.Accept(ctx, userID, int64(num(data, "call_id")))
		return err
	case "call.decline":
		return s.Decline(ctx, userID, int64(num(data, "call_id")))
	case "call.end":
		_, err := s.End(ctx, userID, int64(num(data, "call_id")), str(data, "reason", "hangup"))
		return err
	case "call.offer", "call.answer", "call.ice", "call.video-offer", "call.video-answer":
		return s.Relay(ctx, userID, int64(num(data, "call_id")), typ, data)
	default:
		return utils.BadRequest("unknown event type: " + typ)
	}
}

// watchRing marks a call missed when nobody answers in time.
func (s *Service) watchRing(callID int64) {
	time.AfterFunc(RingTimeout, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		s.mu.Lock()
		defer s.mu.Unlock()

		c, err := s.repo.Get(ctx, callID)
		if err != nil || c.Status != StatusRinging {
			return
		}
		now := time.Now().UTC()
		c.Status = StatusMissed
		c.EndedAt = &now
		if err := s.repo.Update(ctx, c); err != nil {
			return
		}
		s.emit(c, "call.ended", c.CallerID, map[string]any{"by": int64(0), "reason": "no answer"})
		s.emit(c, "call.ended", c.CalleeID, map[string]any{"by": int64(0), "reason": "no answer"})
		s.record(ctx, "call.missed", c)
	})
}

// emit pushes an event about a call to one participant.
func (s *Service) emit(c *Call, typ string, userID int64, extra map[string]any) {
	data := map[string]any{"call": *s.decorate(c, userID)}
	for k, v := range extra {
		data[k] = v
	}
	if s.hub != nil {
		s.hub.SendToUser(userID, realtime.NewEvent(typ, data))
	}
}

// decorate adds viewer relative fields for the JSON payload.
func (s *Service) decorate(c *Call, forUser int64) *Call {
	out := *c
	out.PeerID = c.Peer(forUser)
	if forUser == c.CallerID {
		out.Direction = "outbound"
	} else {
		out.Direction = "inbound"
	}
	return &out
}

// record stores an audit event and publishes it to the message bus.
func (s *Service) record(ctx context.Context, typ string, c *Call) {
	payload := map[string]any{
		"call_id": c.ID,
		"caller":  c.CallerID,
		"callee":  c.CalleeID,
		"kind":    c.Kind,
		"status":  c.Status,
	}
	if s.events != nil {
		_ = s.events.Append(ctx, typ, "", payload)
	}
	kafka.PublishSafe(ctx, s.pub, typ, payload)
}

func num(data map[string]any, key string) float64 {
	if v, ok := data[key].(float64); ok {
		return v
	}
	return 0
}

func str(data map[string]any, key, def string) string {
	if v, ok := data[key].(string); ok && v != "" {
		return v
	}
	return def
}
