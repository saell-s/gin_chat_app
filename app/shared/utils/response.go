package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type envelope struct {
	Data any    `json:"data,omitempty"`
	Page *Pager `json:"page,omitempty"`
}

type Pager struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

// OK writes a 200 response with a data payload.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{Data: data})
}

// Created writes a 201 response with a data payload.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, envelope{Data: data})
}

// Fail writes an error response with the given status.
func Fail(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": gin.H{"message": message, "status": status}})
}

// Abort maps any error onto its HTTP status and writes the response.
func Abort(c *gin.Context, err error) {
	status := StatusOf(err)
	message := MessageOf(err)
	if status == http.StatusInternalServerError {
		// Log the underlying cause, never expose it to clients.
		_ = err
	}
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"message": message, "status": status}})
}

// Paginated writes a 200 list response including pagination metadata.
func Paginated[T any](c *gin.Context, items []T, page, pageSize int, total int64) {
	pages := int64(0)
	if pageSize > 0 {
		pages = (total + int64(pageSize) - 1) / int64(pageSize)
	}
	c.JSON(http.StatusOK, envelope{
		Data: items,
		Page: &Pager{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: pages,
		},
	})
}
