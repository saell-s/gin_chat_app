package call

import "time"

// Call kinds.
const (
	KindAudio = "audio"
	KindVideo = "video"
)

// Call statuses.
const (
	StatusRinging  = "ringing"
	StatusActive   = "active"
	StatusDeclined = "declined"
	StatusMissed   = "missed"
	StatusEnded    = "ended"
	StatusCancelled = "cancelled"
)

// RingTimeout is how long a call rings before it becomes missed.
const RingTimeout = 45 * time.Second

// Call is a single audio/video session.
type Call struct {
	ID              int64      `json:"id"`
	CallerID        int64      `json:"caller_id"`
	CalleeID        int64      `json:"callee_id"`
	ConversationID  int64      `json:"conversation_id"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	StartedAt       time.Time  `json:"started_at"`
	AnsweredAt      *time.Time `json:"answered_at,omitempty"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	DurationSeconds int64      `json:"duration_seconds"`
	CreatedAt       time.Time  `json:"created_at"`

	// Derived fields for the requesting user.
	PeerID    int64  `json:"peer_id"`
	Direction string `json:"direction"` // inbound | outbound
}

// CanControl reports whether userID may accept/decline this call.
func (c *Call) IsParticipant(userID int64) bool {
	return userID == c.CallerID || userID == c.CalleeID
}

// Peer returns the other party from userID's perspective.
func (c *Call) Peer(userID int64) int64 {
	if userID == c.CallerID {
		return c.CalleeID
	}
	return c.CallerID
}
