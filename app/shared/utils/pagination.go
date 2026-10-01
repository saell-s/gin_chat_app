package utils

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

type PageParams struct {
	Page     int
	PageSize int
}

func (p PageParams) Offset() int { return (p.Page - 1) * p.PageSize }
func (p PageParams) Limit() int  { return p.PageSize }

// ParsePage reads ?page= and ?page_size= with sane bounds.
func ParsePage(c *gin.Context) PageParams {
	page := parseInt(c.Query("page"), DefaultPage)
	pageSize := parseInt(c.Query("page_size"), DefaultPageSize)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return PageParams{Page: page, PageSize: pageSize}
}

// ParseDate reads an optional RFC3339 date query parameter.
func ParseDate(c *gin.Context, key string) (string, bool) {
	v := c.Query(key)
	if v == "" {
		return "", false
	}
	return v, true
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
