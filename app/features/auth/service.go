package auth

import (
	"context"
	"errors"
	"time"

	"gin/app/features/user"
	"gin/app/shared/db"
	"gin/app/shared/kafka"
	"gin/app/shared/security"
	"gin/app/shared/utils"
)

// Service implements the authentication use cases.
type Service struct {
	users  *user.Service
	tokens TokenRepository
	tm     *security.TokenManager
	events db.EventRepository
	pub    kafka.Publisher
}

func NewService(users *user.Service, tokens TokenRepository, tm *security.TokenManager, events db.EventRepository, pub kafka.Publisher) *Service {
	return &Service{users: users, tokens: tokens, tm: tm, events: events, pub: pub}
}

// Register creates an account and issues its first token pair.
func (s *Service) Register(ctx context.Context, in user.RegisterInput) (*TokenPair, error) {
	u, err := s.users.Register(ctx, in)
	if err != nil {
		return nil, err
	}
	return s.issue(ctx, u)
}

// Login exchanges credentials for a token pair.
func (s *Service) Login(ctx context.Context, in LoginInput) (*TokenPair, error) {
	u, err := s.users.GetByEmail(ctx, in.Email)
	if err != nil {
		return nil, utils.Unauthorized("invalid email or password")
	}
	if !security.VerifyPassword(u.PasswordHash, in.Password) {
		return nil, utils.Unauthorized("invalid email or password")
	}
	if u.Status != user.StatusActive {
		return nil, utils.Forbidden("account is not active")
	}
	pair, err := s.issue(ctx, u)
	if err != nil {
		return nil, err
	}
	_ = s.events.Append(ctx, "auth.login", u.Email, map[string]any{"user_id": u.ID})
	return pair, nil
}

// Refresh rotates a refresh token and mints a new access token.
func (s *Service) Refresh(ctx context.Context, raw string) (*TokenPair, error) {
	hash := security.HashToken(raw)
	stored, err := s.tokens.FindByHash(ctx, hash)
	if err != nil {
		return nil, utils.Unauthorized("invalid refresh token")
	}
	now := time.Now().UTC()
	if !stored.Active(now) {
		_ = s.tokens.Revoke(ctx, stored.ID)
		return nil, utils.Unauthorized("refresh token expired or revoked")
	}

	u, err := s.users.GetByID(ctx, stored.UserID)
	if err != nil {
		return nil, utils.Unauthorized("invalid refresh token")
	}
	if u.Status != user.StatusActive {
		return nil, utils.Forbidden("account is not active")
	}

	// Rotation: the presented token can never be used twice.
	if err := s.tokens.Revoke(ctx, stored.ID); err != nil && !errors.Is(err, utils.ErrNotFound) {
		return nil, utils.Internal(err)
	}
	return s.issue(ctx, u)
}

// Logout revokes the presented refresh token.
func (s *Service) Logout(ctx context.Context, raw string) error {
	stored, err := s.tokens.FindByHash(ctx, security.HashToken(raw))
	if err != nil {
		// Already unknown: treat logout as idempotent success.
		return nil
	}
	if err := s.tokens.Revoke(ctx, stored.ID); err != nil && !errors.Is(err, utils.ErrNotFound) {
		return utils.Internal(err)
	}
	_ = s.events.Append(ctx, "auth.logout", "", map[string]any{"user_id": stored.UserID})
	return nil
}

// Me returns the authenticated account.
func (s *Service) Me(ctx context.Context, id int64) (*user.User, error) {
	return s.users.GetByID(ctx, id)
}

// issue mints an access token and persists a fresh refresh token.
func (s *Service) issue(ctx context.Context, u *user.User) (*TokenPair, error) {
	access, exp, err := s.tm.GenerateAccess(u.ID, u.Email, u.Role)
	if err != nil {
		return nil, utils.Internal(err)
	}
	raw, hash, err := security.NewRefreshToken()
	if err != nil {
		return nil, utils.Internal(err)
	}
	now := time.Now().UTC()
	t := &RefreshToken{
		UserID:    u.ID,
		TokenHash: hash,
		ExpiresAt: now.Add(s.tm.RefreshTTL()),
		CreatedAt: now,
	}
	if err := s.tokens.Save(ctx, t); err != nil {
		return nil, utils.Internal(err)
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: raw,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.tm.AccessTTL().Seconds()),
		ExpiresAt:    exp.UTC(),
		RefreshAt:    t.ExpiresAt,
		User:         u,
	}, nil
}
