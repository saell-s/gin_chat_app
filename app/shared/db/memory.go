package db

import (
	"sync"
	"time"
)

// Row types shared by every feature repository when the memory backend is active.

type UserRow struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	Role         string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type TokenRow struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
	RevokedAt *time.Time
}

type OrderRow struct {
	ID          int64
	UserID      int64
	Product     string
	AmountCents int64
	Currency    string
	Status      string
	CreatedAt   time.Time
}

type EventRow struct {
	ID        int64
	Type      string
	Actor     string
	Payload   string
	CreatedAt time.Time
}

// Memory is a thread safe in-process store used when postgres is not configured.
type Memory struct {
	mu          sync.RWMutex
	userSeq     int64
	tokenSeq    int64
	orderSeq    int64
	eventSeq    int64
	convSeq     int64
	partSeq     int64
	messageSeq  int64
	callSeq     int64
	Users       []UserRow
	Tokens      []TokenRow
	Orders      []OrderRow
	Events      []EventRow
	Conversations []ConversationRow
	Participants  []ParticipantRow
	Messages      []MessageRow
	Calls         []CallRow
}

func NewMemory() *Memory {
	return &Memory{
		Users:       make([]UserRow, 0, 8),
		Tokens:      make([]TokenRow, 0, 8),
		Orders:      make([]OrderRow, 0, 64),
		Events:      make([]EventRow, 0, 32),
		Conversations: make([]ConversationRow, 0, 8),
		Participants:  make([]ParticipantRow, 0, 16),
		Messages:      make([]MessageRow, 0, 64),
		Calls:         make([]CallRow, 0, 8),
	}
}

func (m *Memory) Close() {}

func (m *Memory) nextID(counter *int64) int64 {
	*counter++
	return *counter
}

// ---- users ----

func (m *Memory) InsertUser(u UserRow) UserRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	u.ID = m.nextID(&m.userSeq)
	now := time.Now().UTC()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	m.Users = append(m.Users, u)
	return u
}

func (m *Memory) UpdateUser(u UserRow) (UserRow, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Users {
		if m.Users[i].ID == u.ID {
			u.CreatedAt = m.Users[i].CreatedAt
			u.UpdatedAt = time.Now().UTC()
			m.Users[i] = u
			return u, true
		}
	}
	return UserRow{}, false
}

func (m *Memory) DeleteUser(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Users {
		if m.Users[i].ID == id {
			m.Users = append(m.Users[:i], m.Users[i+1:]...)
			return true
		}
	}
	return false
}

func (m *Memory) UserSnapshot() []UserRow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]UserRow, len(m.Users))
	copy(out, m.Users)
	return out
}

func (m *Memory) GetUser(id int64) (UserRow, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.Users {
		if u.ID == id {
			return u, true
		}
	}
	return UserRow{}, false
}

func (m *Memory) GetUserByEmail(email string) (UserRow, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.Users {
		if u.Email == email {
			return u, true
		}
	}
	return UserRow{}, false
}

// ---- refresh tokens ----

func (m *Memory) InsertToken(t TokenRow) TokenRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	t.ID = m.nextID(&m.tokenSeq)
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	m.Tokens = append(m.Tokens, t)
	return t
}

func (m *Memory) GetTokenByHash(hash string) (TokenRow, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, t := range m.Tokens {
		if t.TokenHash == hash {
			return t, true
		}
	}
	return TokenRow{}, false
}

func (m *Memory) RevokeToken(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Tokens {
		if m.Tokens[i].ID == id && m.Tokens[i].RevokedAt == nil {
			now := time.Now().UTC()
			m.Tokens[i].RevokedAt = &now
			return true
		}
	}
	return false
}

func (m *Memory) RevokeUserTokens(userID int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	var n int64
	for i := range m.Tokens {
		if m.Tokens[i].UserID == userID && m.Tokens[i].RevokedAt == nil {
			m.Tokens[i].RevokedAt = &now
			n++
		}
	}
	return n
}

// ---- orders ----

func (m *Memory) InsertOrder(o OrderRow) OrderRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	o.ID = m.nextID(&m.orderSeq)
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}
	m.Orders = append(m.Orders, o)
	return o
}

func (m *Memory) OrderSnapshot() []OrderRow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]OrderRow, len(m.Orders))
	copy(out, m.Orders)
	return out
}

// ---- events ----

func (m *Memory) InsertEvent(e EventRow) EventRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.ID = m.nextID(&m.eventSeq)
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	m.Events = append(m.Events, e)
	return e
}

func (m *Memory) EventSnapshot() []EventRow {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]EventRow, len(m.Events))
	copy(out, m.Events)
	return out
}
