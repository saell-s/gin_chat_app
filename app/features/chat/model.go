package chat

import (
	"time"

	"gin/app/features/user"
)

// Conversation kinds.
const (
	TypeDirect = "direct"
	TypeGroup  = "group"
)

// Message kinds.
const (
	KindText   = "text"
	KindSystem = "system"
)

// ConversationView is the client facing conversation summary.
type ConversationView struct {
	ID              int64          `json:"id"`
	Type            string         `json:"type"`
	Peer            *user.User     `json:"peer,omitempty"`
	LastMessage     *Message       `json:"last_message,omitempty"`
	Unread          int64          `json:"unread"`
	LastReadMessage int64          `json:"last_read_message_id"`
	UpdatedAt       time.Time      `json:"updated_at"`
	CreatedAt       time.Time      `json:"created_at"`
}

// Message is a single chat message.
type Message struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	SenderID       int64     `json:"sender_id"`
	Kind           string    `json:"kind"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}

// CreateConversationInput starts (or reuses) a direct conversation.
type CreateConversationInput struct {
	UserID int64 `json:"user_id" binding:"required"`
}

// SendMessageInput posts a message.
type SendMessageInput struct {
	Body string `json:"body" binding:"required,min=1,max=4000"`
}

// ReadInput advances the read cursor.
type ReadInput struct {
	LastReadID int64 `json:"last_read_id" binding:"required"`
}

// ConversationRecord is the persisted conversation plus the viewer's state.
type ConversationRecord struct {
	ID                int64
	Type              string
	PeerID            int64
	LastReadMessageID int64
	LastMessageAt     *time.Time
	CreatedAt         time.Time
}
