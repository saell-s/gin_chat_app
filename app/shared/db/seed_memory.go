package db

import (
	"math/rand/v2"
	"time"
)

// SeedMemory populates the in-memory store with demo data.
func SeedMemory(m *Memory, hash func(string) (string, error)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	empty := len(m.Users) == 0
	m.mu.Unlock()
	if !empty {
		return
	}

	type seedUser struct {
		email    string
		password string
		name     string
		role     string
	}
	seeds := []seedUser{
		{"admin@example.com", "Admin123!", "Ada Admin", "admin"},
		{"editor@example.com", "Editor123!", "Evan Editor", "editor"},
		{"user@example.com", "User12345!", "Uma User", "user"},
	}

	ids := make([]int64, 0, len(seeds))
	for _, s := range seeds {
		h, err := hash(s.password)
		if err != nil {
			continue
		}
		row := m.InsertUser(UserRow{
			Email:        s.email,
			PasswordHash: h,
			Name:         s.name,
			Role:         s.role,
			Status:       "active",
		})
		ids = append(ids, row.ID)
	}

	products := []string{"Pro Plan", "Team Plan", "Starter Pack", "Add-on Seats", "Support Bundle"}
	statuses := []string{"completed", "completed", "completed", "refunded", "pending"}
	now := time.Now().UTC()
	for i := 0; i < 60; i++ {
		m.InsertOrder(OrderRow{
			UserID:      ids[rand.IntN(len(ids))],
			Product:     products[rand.IntN(len(products))],
			AmountCents: int64(rand.IntN(500)+10) * 100,
			Currency:    "USD",
			Status:      statuses[rand.IntN(len(statuses))],
			CreatedAt:   now.Add(-time.Duration(rand.IntN(30*24)) * time.Hour),
		})
	}

	for i, e := range []struct{ typ, actor string }{
		{"user.registered", "admin@example.com"},
		{"order.completed", "system"},
		{"report.generated", "admin@example.com"},
	} {
		m.InsertEvent(EventRow{
			Type:      e.typ,
			Actor:     e.actor,
			Payload:   `{"source":"seed"}`,
			CreatedAt: now.Add(-time.Duration(len(seeds)-i) * time.Hour),
		})
	}

	if len(ids) < 2 {
		return
	}
	// Demo conversation between admin and editor, plus one finished call.
	conv := m.InsertConversation(ConversationRow{Type: "direct", LastMessageAt: now})
	for _, uid := range []int64{ids[0], ids[1]} {
		m.InsertParticipant(ParticipantRow{ConversationID: conv.ID, UserID: uid})
	}
	seedTexts := []struct {
		senderIdx int
		text      string
		agoMin    int
	}{
		{0, "Hi Evan, the messaging beta is live on the API.", 120},
		{1, "Nice. I will check receipts and typing indicators.", 115},
		{0, "Calling runs over WebRTC with WS signaling.", 110},
		{1, "I will test an audio call after lunch.", 105},
	}
	for _, s := range seedTexts {
		m.InsertMessage(MessageRow{
			ConversationID: conv.ID,
			SenderID:       ids[s.senderIdx],
			Kind:           "text",
			Body:           s.text,
			CreatedAt:      now.Add(-time.Duration(s.agoMin) * time.Minute),
		})
	}
	answered := now.Add(-26 * time.Minute)
	m.InsertCall(CallRow{
		CallerID:       ids[1],
		CalleeID:       ids[0],
		ConversationID: conv.ID,
		Kind:           "video",
		Status:         "ended",
		StartedAt:      answered,
		AnsweredAt:     &answered,
		EndedAt:        ptrTime(answered.Add(132 * time.Second)),
		DurationSeconds: 132,
	})
}

func ptrTime(t time.Time) *time.Time { return &t }
