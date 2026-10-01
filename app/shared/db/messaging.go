package db

import "time"

// Messaging and calling rows.

type ConversationRow struct {
	ID            int64
	Type          string // direct | group
	LastMessageAt time.Time
	CreatedAt     time.Time
}

type ParticipantRow struct {
	ConversationID    int64
	UserID            int64
	LastReadMessageID int64
	LastReadAt        time.Time
	CreatedAt         time.Time
}

type MessageRow struct {
	ID             int64
	ConversationID int64
	SenderID       int64
	Kind           string // text | system
	Body           string
	CreatedAt      time.Time
}

type CallRow struct {
	ID              int64
	CallerID        int64
	CalleeID        int64
	ConversationID  int64
	Kind            string // audio | video
	Status          string // ringing | active | declined | missed | ended
	StartedAt       time.Time
	AnsweredAt      *time.Time
	EndedAt         *time.Time
	DurationSeconds int64
	CreatedAt       time.Time
}

// ---- conversations ----

func (m *Memory) InsertConversation(c ConversationRow) ConversationRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.ID = m.nextID(&m.convSeq)
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	m.Conversations = append(m.Conversations, c)
	return c
}

func (m *Memory) GetConversation(id int64) (ConversationRow, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.Conversations {
		if c.ID == id {
			return c, true
		}
	}
	return ConversationRow{}, false
}

func (m *Memory) UpdateConversationLastMessage(id int64, at time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Conversations {
		if m.Conversations[i].ID == id {
			m.Conversations[i].LastMessageAt = at
			return true
		}
	}
	return false
}

func (m *Memory) ConversationSnapshot() []ConversationRow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ConversationRow, len(m.Conversations))
	copy(out, m.Conversations)
	return out
}

// ---- participants ----

func (m *Memory) InsertParticipant(p ParticipantRow) ParticipantRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.LastReadAt.IsZero() {
		p.LastReadAt = p.CreatedAt
	}
	m.Participants = append(m.Participants, p)
	return p
}

func (m *Memory) ParticipantSnapshot() []ParticipantRow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ParticipantRow, len(m.Participants))
	copy(out, m.Participants)
	return out
}

func (m *Memory) GetParticipant(conversationID, userID int64) (ParticipantRow, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.Participants {
		if p.ConversationID == conversationID && p.UserID == userID {
			return p, true
		}
	}
	return ParticipantRow{}, false
}

// SetParticipantRead advances the read cursor when messageID is newer.
func (m *Memory) SetParticipantRead(conversationID, userID, messageID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Participants {
		if m.Participants[i].ConversationID == conversationID && m.Participants[i].UserID == userID {
			if messageID > m.Participants[i].LastReadMessageID {
				m.Participants[i].LastReadMessageID = messageID
				m.Participants[i].LastReadAt = time.Now().UTC()
			}
			return true
		}
	}
	return false
}

// ---- messages ----

func (m *Memory) InsertMessage(msg MessageRow) MessageRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg.ID = m.nextID(&m.messageSeq)
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	m.Messages = append(m.Messages, msg)
	return msg
}

func (m *Memory) GetMessage(id int64) (MessageRow, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, msg := range m.Messages {
		if msg.ID == id {
			return msg, true
		}
	}
	return MessageRow{}, false
}

// MessageSnapshot returns messages of one conversation ordered by id ascending.
func (m *Memory) MessageSnapshot(conversationID int64) []MessageRow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]MessageRow, 0, 16)
	for _, msg := range m.Messages {
		if msg.ConversationID == conversationID {
			out = append(out, msg)
		}
	}
	sortMessages(out)
	return out
}

func sortMessages(rows []MessageRow) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].ID < rows[j-1].ID; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// ---- calls ----

func (m *Memory) InsertCall(c CallRow) CallRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.ID = m.nextID(&m.callSeq)
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if c.StartedAt.IsZero() {
		c.StartedAt = c.CreatedAt
	}
	m.Calls = append(m.Calls, c)
	return c
}

func (m *Memory) GetCall(id int64) (CallRow, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.Calls {
		if c.ID == id {
			return c, true
		}
	}
	return CallRow{}, false
}

func (m *Memory) UpdateCall(c CallRow) (CallRow, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Calls {
		if m.Calls[i].ID == c.ID {
			m.Calls[i] = c
			return c, true
		}
	}
	return CallRow{}, false
}

func (m *Memory) CallSnapshot() []CallRow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]CallRow, len(m.Calls))
	copy(out, m.Calls)
	return out
}
