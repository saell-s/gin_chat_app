package auth

import (
	"time"

	"gin/app/features/user"
)

// LoginInput is the credentials payload.
type LoginInput struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,min=1,max=72"`
}

// RefreshInput carries an issued refresh token.
type RefreshInput struct {
	RefreshToken string `json:"refresh_token" binding:"required,min=10,max=256"`
}

// TokenPair is returned on successful authentication.
type TokenPair struct {
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
	TokenType    string     `json:"token_type"`
	ExpiresIn    int64      `json:"expires_in"`
	ExpiresAt    time.Time  `json:"expires_at"`
	RefreshAt    time.Time  `json:"refresh_expires_at"`
	User         *user.User `json:"user"`
}

// RefreshToken is the persisted refresh credential (stored hashed).
type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
	RevokedAt *time.Time
}

func (t *RefreshToken) Active(now time.Time) bool {
	return t.RevokedAt == nil && t.ExpiresAt.After(now)
}
