package user

import "time"

// Roles recognised across the API.
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleUser   = "user"
)

// Statuses recognised across the API.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

// User is the canonical user record. PasswordHash is never serialised.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RegisterInput is the payload for account creation.
type RegisterInput struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	Name     string `json:"name" binding:"required,min=1,max=120"`
}

// UpdateProfileInput is the payload a user may apply to themselves.
type UpdateProfileInput struct {
	Name  *string `json:"name" binding:"omitempty,min=1,max=120"`
	Email *string `json:"email" binding:"omitempty,email,max=254"`
}

// AdminUpdateInput is the payload an administrator may apply to any user.
type AdminUpdateInput struct {
	Name   *string `json:"name" binding:"omitempty,min=1,max=120"`
	Role   *string `json:"role" binding:"omitempty,oneof=admin editor user"`
	Status *string `json:"status" binding:"omitempty,oneof=active suspended"`
}

// ChangePasswordInput is the payload for changing one's own password.
type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password" binding:"required,min=8,max=72"`
	NewPassword     string `json:"new_password" binding:"required,min=8,max=72"`
}

// ListFilter scopes user listings.
type ListFilter struct {
	Role     string
	Status   string
	Query    string
	Page     int
	PageSize int
}
