package chat

import (
	"context"
	"time"

	"gin/app/shared/db"
	"gin/app/shared/utils"
)

// Repository persists conversations, participants and messages.
type Repository interface {
	CreateConversation(ctx context.Context, kind string, userIDs []int64) (*ConversationRecord, error)
	FindDirect(ctx context.Context, a, b int64) (*ConversationRecord, error)
	ListConversations(ctx context.Context, userID int64) ([]ConversationRecord, error)
	IsParticipant(ctx context.Context, conversationID, userID int64) (bool, error)
	ParticipantIDs(ctx context.Context, conversationID int64) ([]int64, error)

	AddMessage(ctx context.Context, m *Message) error
	ListMessages(ctx context.Context, conversationID, beforeID int64, limit int) ([]Message, error)
	LastMessage(ctx context.Context, conversationID int64) (*Message, bool, error)
	UnreadCount(ctx context.Context, conversationID, userID, lastReadID int64) (int64, error)
	MarkRead(ctx context.Context, conversationID, userID, messageID int64) error
	TouchConversation(ctx context.Context, conversationID int64, at time.Time) error
}

// NewRepository selects the backend implementation.
func NewRepository(d *db.Database) Repository {
	if d.IsPostgres() {
		return &pgRepo{q: d.Pool}
	}
	return &memRepo{mem: d.Mem}
}

// ---- memory ----

type memRepo struct{ mem *db.Memory }

func (r *memRepo) CreateConversation(ctx context.Context, kind string, userIDs []int64) (*ConversationRecord, error) {
	conv := r.mem.InsertConversation(db.ConversationRow{Type: kind})
	for _, uid := range userIDs {
		r.mem.InsertParticipant(db.ParticipantRow{ConversationID: conv.ID, UserID: uid})
	}
	return &ConversationRecord{ID: conv.ID, Type: conv.Type, CreatedAt: conv.CreatedAt}, nil
}

func (r *memRepo) FindDirect(ctx context.Context, a, b int64) (*ConversationRecord, error) {
	participants := r.mem.ParticipantSnapshot()
	byConv := map[int64][]int64{}
	for _, p := range participants {
		byConv[p.ConversationID] = append(byConv[p.ConversationID], p.UserID)
	}
	for _, conv := range r.mem.ConversationSnapshot() {
		if conv.Type != TypeDirect {
			continue
		}
		uids := byConv[conv.ID]
		if len(uids) != 2 {
			continue
		}
		if !contains(uids, a) || !contains(uids, b) {
			continue
		}
		return r.toRecord(conv, a, participants), nil
	}
	return nil, utils.ErrNotFound
}

func contains(list []int64, v int64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (r *memRepo) toRecord(conv db.ConversationRow, viewerID int64, participants []db.ParticipantRow) *ConversationRecord {
	rec := &ConversationRecord{
		ID:        conv.ID,
		Type:      conv.Type,
		CreatedAt: conv.CreatedAt,
	}
	if !conv.LastMessageAt.IsZero() {
		t := conv.LastMessageAt
		rec.LastMessageAt = &t
	}
	for _, p := range participants {
		if p.ConversationID != conv.ID {
			continue
		}
		if p.UserID == viewerID {
			rec.LastReadMessageID = p.LastReadMessageID
		} else {
			rec.PeerID = p.UserID
		}
	}
	return rec
}

func (r *memRepo) ListConversations(ctx context.Context, userID int64) ([]ConversationRecord, error) {
	participants := r.mem.ParticipantSnapshot()
	mine := make([]db.ParticipantRow, 0, 8)
	for _, p := range participants {
		if p.UserID == userID {
			mine = append(mine, p)
		}
	}
	convs := r.mem.ConversationSnapshot()
	out := make([]ConversationRecord, 0, len(mine))
	for _, p := range mine {
		for _, conv := range convs {
			if conv.ID == p.ConversationID {
				rec := r.toRecord(conv, userID, participants)
				out = append(out, *rec)
				break
			}
		}
	}
	return out, nil
}

func (r *memRepo) IsParticipant(ctx context.Context, conversationID, userID int64) (bool, error) {
	_, ok := r.mem.GetParticipant(conversationID, userID)
	return ok, nil
}

func (r *memRepo) ParticipantIDs(ctx context.Context, conversationID int64) ([]int64, error) {
	out := make([]int64, 0, 4)
	for _, p := range r.mem.ParticipantSnapshot() {
		if p.ConversationID == conversationID {
			out = append(out, p.UserID)
		}
	}
	return out, nil
}

func (r *memRepo) AddMessage(ctx context.Context, m *Message) error {
	row := r.mem.InsertMessage(db.MessageRow{
		ConversationID: m.ConversationID,
		SenderID:       m.SenderID,
		Kind:           m.Kind,
		Body:           m.Body,
		CreatedAt:      m.CreatedAt,
	})
	m.ID = row.ID
	m.CreatedAt = row.CreatedAt
	return nil
}

func (r *memRepo) ListMessages(ctx context.Context, conversationID, beforeID int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := r.mem.MessageSnapshot(conversationID) // ascending
	out := make([]Message, 0, limit)
	for i := len(rows) - 1; i >= 0; i-- {
		if beforeID > 0 && rows[i].ID >= beforeID {
			continue
		}
		out = append(out, toMessage(rows[i]))
		if len(out) >= limit {
			break
		}
	}
	// Return oldest first for rendering.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (r *memRepo) LastMessage(ctx context.Context, conversationID int64) (*Message, bool, error) {
	rows := r.mem.MessageSnapshot(conversationID)
	if len(rows) == 0 {
		return nil, false, nil
	}
	m := toMessage(rows[len(rows)-1])
	return &m, true, nil
}

func (r *memRepo) UnreadCount(ctx context.Context, conversationID, userID, lastReadID int64) (int64, error) {
	var n int64
	for _, row := range r.mem.MessageSnapshot(conversationID) {
		if row.ID > lastReadID && row.SenderID != userID {
			n++
		}
	}
	return n, nil
}

func (r *memRepo) MarkRead(ctx context.Context, conversationID, userID, messageID int64) error {
	if !r.mem.SetParticipantRead(conversationID, userID, messageID) {
		return utils.ErrNotFound
	}
	return nil
}

func (r *memRepo) TouchConversation(ctx context.Context, conversationID int64, at time.Time) error {
	r.mem.UpdateConversationLastMessage(conversationID, at)
	return nil
}

func toMessage(row db.MessageRow) Message {
	return Message{
		ID:             row.ID,
		ConversationID: row.ConversationID,
		SenderID:       row.SenderID,
		Kind:           row.Kind,
		Body:           row.Body,
		CreatedAt:      row.CreatedAt,
	}
}

// ---- postgres ----

type pgRepo struct{ q db.Pgx }

func (r *pgRepo) CreateConversation(ctx context.Context, kind string, userIDs []int64) (*ConversationRecord, error) {
	var rec ConversationRecord
	err := r.q.QueryRow(ctx,
		`INSERT INTO conversations (type) VALUES ($1) RETURNING id, type, created_at`, kind,
	).Scan(&rec.ID, &rec.Type, &rec.CreatedAt)
	if err != nil {
		return nil, err
	}
	for _, uid := range userIDs {
		if _, err := r.q.Exec(ctx,
			`INSERT INTO conversation_participants (conversation_id, user_id) VALUES ($1,$2)
			 ON CONFLICT DO NOTHING`, rec.ID, uid); err != nil {
			return nil, err
		}
	}
	return &rec, nil
}

func (r *pgRepo) FindDirect(ctx context.Context, a, b int64) (*ConversationRecord, error) {
	rec := &ConversationRecord{Type: TypeDirect, PeerID: b}
	err := r.q.QueryRow(ctx,
		`SELECT c.id, c.type, c.last_message_at, c.created_at,
		        (SELECT p2.user_id FROM conversation_participants p2
		         WHERE p2.conversation_id = c.id AND p2.user_id <> $1),
		        (SELECT p1.last_read_message_id FROM conversation_participants p1
		         WHERE p1.conversation_id = c.id AND p1.user_id = $1)
		 FROM conversations c
		 JOIN conversation_participants pa ON pa.conversation_id = c.id AND pa.user_id = $1
		 JOIN conversation_participants pb ON pb.conversation_id = c.id AND pb.user_id = $2
		 WHERE c.type = 'direct'
		 LIMIT 1`, a, b,
	).Scan(&rec.ID, &rec.Type, &rec.LastMessageAt, &rec.CreatedAt, &rec.PeerID, &rec.LastReadMessageID)
	if err != nil {
		if err == db.ErrNoRows {
			return nil, utils.ErrNotFound
		}
		return nil, err
	}
	return rec, nil
}

const conversationSelect = `
	SELECT c.id, c.type, c.last_message_at, c.created_at,
	       (SELECT p2.user_id FROM conversation_participants p2
	        WHERE p2.conversation_id = c.id AND p2.user_id <> $1 LIMIT 1),
	       (SELECT p1.last_read_message_id FROM conversation_participants p1
	        WHERE p1.conversation_id = c.id AND p1.user_id = $1)
	FROM conversations c
	JOIN conversation_participants p ON p.conversation_id = c.id AND p.user_id = $1`

func (r *pgRepo) ListConversations(ctx context.Context, userID int64) ([]ConversationRecord, error) {
	rows, err := r.q.Query(ctx, conversationSelect+`
		ORDER BY COALESCE(c.last_message_at, c.created_at) DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ConversationRecord, 0, 8)
	for rows.Next() {
		rec := ConversationRecord{}
		if err := rows.Scan(&rec.ID, &rec.Type, &rec.LastMessageAt, &rec.CreatedAt, &rec.PeerID, &rec.LastReadMessageID); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *pgRepo) IsParticipant(ctx context.Context, conversationID, userID int64) (bool, error) {
	var exists bool
	err := r.q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM conversation_participants WHERE conversation_id=$1 AND user_id=$2)`,
		conversationID, userID).Scan(&exists)
	return exists, err
}

func (r *pgRepo) ParticipantIDs(ctx context.Context, conversationID int64) ([]int64, error) {
	rows, err := r.q.Query(ctx,
		`SELECT user_id FROM conversation_participants WHERE conversation_id=$1`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0, 4)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *pgRepo) AddMessage(ctx context.Context, m *Message) error {
	kind := m.Kind
	if kind == "" {
		kind = KindText
	}
	return r.q.QueryRow(ctx,
		`INSERT INTO messages (conversation_id, sender_id, kind, body, created_at)
		 VALUES ($1,$2,$3,$4,COALESCE($5, now()))
		 RETURNING id, created_at`,
		m.ConversationID, m.SenderID, kind, m.Body, nullTime(m.CreatedAt),
	).Scan(&m.ID, &m.CreatedAt)
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func (r *pgRepo) ListMessages(ctx context.Context, conversationID, beforeID int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.q.Query(ctx,
		`SELECT id, conversation_id, sender_id, kind, body, created_at
		 FROM messages
		 WHERE conversation_id = $1 AND ($2 = 0 OR id < $2)
		 ORDER BY id DESC
		 LIMIT $3`, conversationID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Message, 0, limit)
	for rows.Next() {
		m := Message{}
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Kind, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to oldest-first order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (r *pgRepo) LastMessage(ctx context.Context, conversationID int64) (*Message, bool, error) {
	m := Message{}
	err := r.q.QueryRow(ctx,
		`SELECT id, conversation_id, sender_id, kind, body, created_at
		 FROM messages WHERE conversation_id = $1 ORDER BY id DESC LIMIT 1`, conversationID,
	).Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Kind, &m.Body, &m.CreatedAt)
	if err != nil {
		if err == db.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &m, true, nil
}

func (r *pgRepo) UnreadCount(ctx context.Context, conversationID, userID, lastReadID int64) (int64, error) {
	var n int64
	err := r.q.QueryRow(ctx,
		`SELECT count(*) FROM messages
		 WHERE conversation_id = $1 AND id > $2 AND sender_id <> $3`,
		conversationID, lastReadID, userID).Scan(&n)
	return n, err
}

func (r *pgRepo) MarkRead(ctx context.Context, conversationID, userID, messageID int64) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE conversation_participants
		 SET last_read_message_id = GREATEST(last_read_message_id, $3), last_read_at = now()
		 WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID, messageID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return utils.ErrNotFound
	}
	return nil
}

func (r *pgRepo) TouchConversation(ctx context.Context, conversationID int64, at time.Time) error {
	_, err := r.q.Exec(ctx,
		`UPDATE conversations SET last_message_at = $2 WHERE id = $1`, conversationID, at)
	return err
}
