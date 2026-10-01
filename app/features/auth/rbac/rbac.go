package rbac

import (
	"github.com/gin-gonic/gin"

	"gin/app/shared/middleware"
	"gin/app/shared/utils"
)

// Role mirrors the user roles stored on the account.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleUser   Role = "user"
)

// rank orders roles from least to most privileged.
var rank = map[Role]int{
	RoleUser:   1,
	RoleEditor: 2,
	RoleAdmin:  3,
}

// ParseRole converts a raw string into a known Role.
func ParseRole(s string) (Role, bool) {
	r := Role(s)
	_, ok := rank[r]
	return r, ok
}

// HasRole reports whether the actor satisfies at least one of the allowed roles.
func HasRole(actor string, allowed ...Role) bool {
	r, ok := ParseRole(actor)
	if !ok {
		return false
	}
	for _, a := range allowed {
		if r == a {
			return true
		}
	}
	return false
}

// AtLeast reports whether the actor's role is at or above the minimum rank.
func AtLeast(actor string, min Role) bool {
	r, ok := ParseRole(actor)
	if !ok {
		return false
	}
	return rank[r] >= rank[min]
}

// Require aborts the request unless the caller's role is at or above the
// most privileged role in the allowed list. Require(rbac.RoleEditor) therefore
// admits editors and admins.
func Require(allowed ...Role) gin.HandlerFunc {
	threshold := RoleUser
	seen := false
	for _, a := range allowed {
		if !seen || rank[a] > rank[threshold] {
			threshold = a
		}
		seen = true
	}

	return func(c *gin.Context) {
		claims, ok := middleware.Claims(c)
		if !ok {
			utils.Abort(c, utils.Unauthorized("authentication required"))
			return
		}
		if !AtLeast(claims.Role, threshold) {
			utils.Abort(c, utils.Forbidden("insufficient role"))
			return
		}
		c.Next()
	}
}
