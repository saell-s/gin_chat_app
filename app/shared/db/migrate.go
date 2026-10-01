package db

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var schema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id BIGSERIAL PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT 'user',
		status TEXT NOT NULL DEFAULT 'active',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS refresh_tokens (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_hash TEXT NOT NULL UNIQUE,
		expires_at TIMESTAMPTZ NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		revoked_at TIMESTAMPTZ
	)`,
	`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id)`,
	`CREATE TABLE IF NOT EXISTS orders (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		product TEXT NOT NULL,
		amount_cents BIGINT NOT NULL,
		currency TEXT NOT NULL DEFAULT 'USD',
		status TEXT NOT NULL DEFAULT 'completed',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at)`,
	`CREATE TABLE IF NOT EXISTS events (
		id BIGSERIAL PRIMARY KEY,
		type TEXT NOT NULL,
		actor TEXT NOT NULL DEFAULT '',
		payload TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at DESC)`,
	`CREATE TABLE IF NOT EXISTS conversations (
		id BIGSERIAL PRIMARY KEY,
		type TEXT NOT NULL DEFAULT 'direct',
		last_message_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS conversation_participants (
		conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		last_read_message_id BIGINT NOT NULL DEFAULT 0,
		last_read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		PRIMARY KEY (conversation_id, user_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_conv_participants_user ON conversation_participants(user_id)`,
	`CREATE TABLE IF NOT EXISTS messages (
		id BIGSERIAL PRIMARY KEY,
		conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		sender_id BIGINT NOT NULL,
		kind TEXT NOT NULL DEFAULT 'text',
		body TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id, id DESC)`,
	`CREATE TABLE IF NOT EXISTS calls (
		id BIGSERIAL PRIMARY KEY,
		caller_id BIGINT NOT NULL,
		callee_id BIGINT NOT NULL,
		conversation_id BIGINT REFERENCES conversations(id) ON DELETE SET NULL,
		kind TEXT NOT NULL DEFAULT 'audio',
		status TEXT NOT NULL DEFAULT 'ringing',
		started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		answered_at TIMESTAMPTZ,
		ended_at TIMESTAMPTZ,
		duration_seconds BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_calls_caller ON calls(caller_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_calls_callee ON calls(callee_id, created_at DESC)`,
}

func migrateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	for i, stmt := range schema {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migration step %d: %w", i+1, err)
		}
	}
	return nil
}

const seededUsers = 3

func seedPostgres(ctx context.Context, pool *pgxpool.Pool, hash func(string) (string, error)) error {
	var n int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	type seedUser struct {
		email    string
		password string
		name     string
		role     string
	}
	users := []seedUser{
		{"admin@example.com", "Admin123!", "Ada Admin", "admin"},
		{"editor@example.com", "Editor123!", "Evan Editor", "editor"},
		{"user@example.com", "User12345!", "Uma User", "user"},
	}

	ids := make([]int64, 0, len(users))
	for _, u := range users {
		h, err := hash(u.password)
		if err != nil {
			return err
		}
		var id int64
		err = pool.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name, role) VALUES ($1,$2,$3,$4) RETURNING id`,
			u.email, h, u.name, u.role,
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("seed user %s: %w", u.email, err)
		}
		ids = append(ids, id)
	}

	products := []string{"Pro Plan", "Team Plan", "Starter Pack", "Add-on Seats", "Support Bundle"}
	statuses := []string{"completed", "completed", "completed", "refunded", "pending"}
	now := time.Now().UTC()
	for i := 0; i < 60; i++ {
		created := now.Add(-time.Duration(rand.IntN(30*24)) * time.Hour)
		_, err := pool.Exec(ctx,
			`INSERT INTO orders (user_id, product, amount_cents, currency, status, created_at)
			 VALUES ($1,$2,$3,'USD',$4,$5)`,
			ids[rand.IntN(len(ids))],
			products[rand.IntN(len(products))],
			int64(rand.IntN(500)+10)*100,
			statuses[rand.IntN(len(statuses))],
			created,
		)
		if err != nil {
			return fmt.Errorf("seed order: %w", err)
		}
	}

	events := []struct{ typ, actor string }{
		{"user.registered", "admin@example.com"},
		{"order.completed", "system"},
		{"report.generated", "admin@example.com"},
	}
	for i, e := range events {
		_, err := pool.Exec(ctx,
			`INSERT INTO events (type, actor, payload, created_at) VALUES ($1,$2,$3,$4)`,
			e.typ, e.actor, `{"source":"seed"}`, now.Add(-time.Duration(len(events)-i)*time.Hour),
		)
		if err != nil {
			return fmt.Errorf("seed event: %w", err)
		}
	}

	// A demo conversation between admin and editor plus one finished call.
	var convID int64
	err := pool.QueryRow(ctx, `INSERT INTO conversations (type, last_message_at) VALUES ('direct', $1) RETURNING id`, now).Scan(&convID)
	if err != nil {
		return fmt.Errorf("seed conversation: %w", err)
	}
	for _, uid := range []int64{ids[0], ids[1]} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO conversation_participants (conversation_id, user_id, last_read_message_id) VALUES ($1,$2,0)`,
			convID, uid); err != nil {
			return fmt.Errorf("seed participant: %w", err)
		}
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
	var lastMsgID int64
	for _, s := range seedTexts {
		if err := pool.QueryRow(ctx,
			`INSERT INTO messages (conversation_id, sender_id, kind, body, created_at)
			 VALUES ($1,$2,'text',$3,$4) RETURNING id`,
			convID, ids[s.senderIdx], s.text, now.Add(-time.Duration(s.agoMin)*time.Minute),
		).Scan(&lastMsgID); err != nil {
			return fmt.Errorf("seed message: %w", err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE conversations SET last_message_at = $1 WHERE id = $2`, now, convID); err != nil {
		return err
	}
	answered := now.Add(-26 * time.Minute)
	if _, err := pool.Exec(ctx,
		`INSERT INTO calls (caller_id, callee_id, conversation_id, kind, status, started_at, answered_at, ended_at, duration_seconds, created_at)
		 VALUES ($1,$2,$3,'video','ended',$4,$5,$6,132,$4)`,
		ids[1], ids[0], convID, answered, answered, answered.Add(132*time.Second),
	); err != nil {
		return fmt.Errorf("seed call: %w", err)
	}

	log.Printf("seeded %d demo users, 60 orders, %d events and a demo chat", seededUsers, len(events))
	return nil
}
