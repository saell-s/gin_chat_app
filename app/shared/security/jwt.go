package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"gin/app/shared/configs"
)

// ErrTokenExpired is returned when a JWT is past its expiry.
var ErrTokenExpired = errors.New("token expired")

// Claims is the access token payload.
type Claims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	Type  string `json:"type"`
	jwt.RegisteredClaims
}

// TokenManager issues and validates access tokens and opaque refresh tokens.
type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	issuer     string
}

func NewTokenManager(cfg configs.JWTConfig) *TokenManager {
	return &TokenManager{
		secret:     []byte(cfg.Secret),
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
		issuer:     cfg.Issuer,
	}
}

func (t *TokenManager) AccessTTL() time.Duration  { return t.accessTTL }
func (t *TokenManager) RefreshTTL() time.Duration { return t.refreshTTL }

// GenerateAccess creates a signed HS256 access token.
func (t *TokenManager) GenerateAccess(userID int64, email, role string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(t.accessTTL)
	claims := Claims{
		Email: email,
		Role:  role,
		Type:  "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    t.issuer,
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// ParseAccess validates a JWT and returns its claims.
func (t *TokenManager) ParseAccess(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", tok.Header["alg"])
		}
		return t.secret, nil
	}, jwt.WithIssuer(t.issuer), jwt.WithExpirationRequired())
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Type != "access" {
		return nil, errors.New("wrong token type")
	}
	return claims, nil
}

// NewRefreshToken returns a random opaque token and its SHA-256 hex digest.
// Only the digest is ever persisted.
func NewRefreshToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// HashToken digests a refresh token for storage/lookup.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
