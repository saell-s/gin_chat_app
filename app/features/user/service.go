package user

import (
	"context"
	"errors"
	"strings"

	"gin/app/shared/db"
	"gin/app/shared/kafka"
	"gin/app/shared/security"
	"gin/app/shared/utils"
)

// Service holds the user use cases.
type Service struct {
	repo   Repository
	events db.EventRepository
	pub    kafka.Publisher

	// OnPasswordChanged and OnDeleted let the auth layer revoke sessions
	// without creating an import cycle.
	OnPasswordChanged func(ctx context.Context, userID int64)
	OnDeleted         func(ctx context.Context, userID int64)
}

func NewService(repo Repository, events db.EventRepository, pub kafka.Publisher) *Service {
	return &Service{repo: repo, events: events, pub: pub}
}

// Register creates a new standard account.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	u := &User{
		Email:  email,
		Name:   strings.TrimSpace(in.Name),
		Role:   RoleUser,
		Status: StatusActive,
	}
	hash, err := security.HashPassword(in.Password)
	if err != nil {
		return nil, utils.Internal(err)
	}
	u.PasswordHash = hash

	if err := s.repo.Create(ctx, u); err != nil {
		if errors.Is(err, utils.ErrConflict) {
			return nil, utils.Conflict("an account with this email already exists")
		}
		return nil, utils.Internal(err)
	}

	_ = s.events.Append(ctx, "user.registered", u.Email, map[string]any{"user_id": u.ID})
	kafka.PublishSafe(ctx, s.pub, "user.registered", map[string]any{
		"user_id": u.ID, "email": u.Email, "role": u.Role,
	})
	return u, nil
}

// GetByID returns one user.
func (s *Service) GetByID(ctx context.Context, id int64) (*User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return nil, utils.NotFound("user not found")
		}
		return nil, utils.Internal(err)
	}
	return u, nil
}

// GetByEmail returns one user by email address.
func (s *Service) GetByEmail(ctx context.Context, email string) (*User, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return nil, utils.NotFound("user not found")
		}
		return nil, utils.Internal(err)
	}
	return u, nil
}

// List returns a page of users plus the total match count.
func (s *Service) List(ctx context.Context, f ListFilter) ([]User, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = utils.DefaultPageSize
	}
	users, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, 0, utils.Internal(err)
	}
	return users, total, nil
}

// UpdateProfile lets a user edit their own name/email.
func (s *Service) UpdateProfile(ctx context.Context, id int64, in UpdateProfileInput) (*User, error) {
	u, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		u.Name = strings.TrimSpace(*in.Name)
	}
	if in.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*in.Email))
		if email != u.Email {
			if _, err := s.repo.GetByEmail(ctx, email); err == nil {
				return nil, utils.Conflict("an account with this email already exists")
			}
		}
		u.Email = email
	}
	if err := s.repo.Update(ctx, u); err != nil {
		if errors.Is(err, utils.ErrConflict) {
			return nil, utils.Conflict("an account with this email already exists")
		}
		return nil, utils.Internal(err)
	}
	_ = s.events.Append(ctx, "user.updated", u.Email, map[string]any{"user_id": u.ID})
	return u, nil
}

// AdminUpdate lets an administrator change name/role/status.
func (s *Service) AdminUpdate(ctx context.Context, id int64, in AdminUpdateInput) (*User, error) {
	u, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		u.Name = strings.TrimSpace(*in.Name)
	}
	if in.Role != nil {
		u.Role = *in.Role
	}
	if in.Status != nil {
		u.Status = *in.Status
	}
	if err := s.repo.Update(ctx, u); err != nil {
		if errors.Is(err, utils.ErrConflict) {
			return nil, utils.Conflict("an account with this email already exists")
		}
		return nil, utils.Internal(err)
	}
	_ = s.events.Append(ctx, "user.updated", "admin", map[string]any{
		"user_id": u.ID, "role": u.Role, "status": u.Status,
	})
	return u, nil
}

// ChangePassword rotates the caller's password after checking the current one.
func (s *Service) ChangePassword(ctx context.Context, id int64, in ChangePasswordInput) error {
	u, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !security.VerifyPassword(u.PasswordHash, in.CurrentPassword) {
		return utils.BadRequest("current password is incorrect")
	}
	hash, err := security.HashPassword(in.NewPassword)
	if err != nil {
		return utils.Internal(err)
	}
	if err := s.repo.SetPassword(ctx, id, hash); err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return utils.NotFound("user not found")
		}
		return utils.Internal(err)
	}
	_ = s.events.Append(ctx, "user.password_changed", u.Email, map[string]any{"user_id": u.ID})
	if s.OnPasswordChanged != nil {
		s.OnPasswordChanged(ctx, id)
	}
	return nil
}

// Delete removes a user.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return utils.NotFound("user not found")
		}
		return utils.Internal(err)
	}
	_ = s.events.Append(ctx, "user.deleted", "admin", map[string]any{"user_id": id})
	if s.OnDeleted != nil {
		s.OnDeleted(ctx, id)
	}
	return nil
}

// CanManage reports whether actor may modify target (self or admin).
func CanManage(actorID int64, actorRole string, targetID int64) bool {
	return actorID == targetID || actorRole == RoleAdmin
}
