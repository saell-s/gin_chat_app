package middleware

import (
	"crypto/rand"
	"encoding/hex"
)

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "req-0"
	}
	return hex.EncodeToString(buf)
}
