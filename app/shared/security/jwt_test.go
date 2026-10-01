package security

import (
	"testing"
	"time"

	"gin/app/shared/configs"
)

func testManager() *TokenManager {
	return NewTokenManager(configs.JWTConfig{
		Secret:     "test-secret",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
		Issuer:     "test",
	})
}

func TestAccessTokenRoundTrip(t *testing.T) {
	tm := testManager()
	token, exp, err := tm.GenerateAccess(42, "a@example.com", "admin")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if time.Until(exp) <= 0 {
		t.Fatalf("expiry should be in the future, got %v", exp)
	}

	claims, err := tm.ParseAccess(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Subject != "42" || claims.Email != "a@example.com" || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.Type != "access" {
		t.Fatalf("expected access token, got %q", claims.Type)
	}
}

func TestAccessTokenRejectsOtherSecret(t *testing.T) {
	tm := testManager()
	token, _, err := tm.GenerateAccess(1, "a@example.com", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	other := NewTokenManager(configs.JWTConfig{Secret: "different", Issuer: "test"})
	if _, err := other.ParseAccess(token); err == nil {
		t.Fatal("expected signature validation to fail")
	}
}

func TestAccessTokenRejectsExpired(t *testing.T) {
	tm := NewTokenManager(configs.JWTConfig{
		Secret:    "test-secret",
		AccessTTL: -time.Minute,
		Issuer:    "test",
	})
	token, _, err := tm.GenerateAccess(1, "a@example.com", "user")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := tm.ParseAccess(token); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestRefreshTokenHashIsStableAndOpaque(t *testing.T) {
	raw, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("new refresh token: %v", err)
	}
	if len(raw) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(raw))
	}
	if hash != HashToken(raw) {
		t.Fatal("hash must be reproducible from the raw token")
	}
	if hash == raw {
		t.Fatal("stored hash must differ from the raw token")
	}

	other, _, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("new refresh token: %v", err)
	}
	if other == raw {
		t.Fatal("tokens must be unique")
	}
}
