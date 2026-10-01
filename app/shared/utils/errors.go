package utils

import (
	"errors"
	"net/http"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

// AppError carries an HTTP status and a client safe message.
type AppError struct {
	Status  int
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

func NewError(status int, message string) *AppError {
	return &AppError{Status: status, Message: message}
}

func BadRequest(message string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Message: message}
}

func Unauthorized(message string) *AppError {
	return &AppError{Status: http.StatusUnauthorized, Message: message}
}

func Forbidden(message string) *AppError {
	return &AppError{Status: http.StatusForbidden, Message: message}
}

func NotFound(message string) *AppError {
	return &AppError{Status: http.StatusNotFound, Message: message}
}

func Conflict(message string) *AppError {
	return &AppError{Status: http.StatusConflict, Message: message}
}

func Internal(err error) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: "internal server error", Err: err}
}

// StatusOf maps an arbitrary error to an HTTP status code.
func StatusOf(err error) int {
	var app *AppError
	if errors.As(err, &app) {
		return app.Status
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

// MessageOf returns a client safe message for any error.
func MessageOf(err error) string {
	var app *AppError
	if errors.As(err, &app) {
		return app.Message
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return "resource not found"
	case errors.Is(err, ErrConflict):
		return "resource already exists"
	case errors.Is(err, ErrUnauthorized):
		return "invalid credentials"
	case errors.Is(err, ErrForbidden):
		return "access denied"
	default:
		return "internal server error"
	}
}
