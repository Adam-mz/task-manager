package auth

import (
	"bytes"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newTestTokenManager(t *testing.T) (*TokenManager, []byte) {
	t.Helper()

	secret := bytes.Repeat([]byte{1}, 64)
	manager, err := NewTokenManager(secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager(): %v", err)
	}

	return manager, secret
}

func validTestClaims() jwt.RegisteredClaims {
	now := time.Now().UTC()

	return jwt.RegisteredClaims{
		Subject:   "42",
		Issuer:    tokenIssuer,
		Audience:  jwt.ClaimStrings{tokenAudience},
		IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
		ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
	}
}

func assertInvalidToken(t *testing.T, manager *TokenManager, token string) {
	t.Helper()

	userID, err := manager.Parse(token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Parse() error = %v, want ErrInvalidToken", err)
	}
	if userID != 0 {
		t.Errorf("Parse() userID = %d, want 0", userID)
	}
}

func TestNewTokenManager(t *testing.T) {
	tests := []struct {
		name      string
		keyLength int
		ttl       time.Duration
		wantErr   bool
	}{
		{"minimum key", 32, time.Minute, false},
		{"longer key", 64, time.Minute, false},
		{"empty key", 0, time.Minute, true},
		{"short key", 31, time.Minute, true},
		{"zero ttl", 32, 0, true},
		{"negative ttl", 32, -time.Minute, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := bytes.Repeat([]byte{1}, tt.keyLength)
			manager, err := NewTokenManager(secret, tt.ttl)

			if (err != nil) != tt.wantErr {
				t.Fatalf("NewTokenManager() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && manager == nil {
				t.Fatal("NewTokenManager() returned nil manager")
			}
		})
	}
}

func TestTokenManagerGenerateAndParse(t *testing.T) {
	manager, _ := newTestTokenManager(t)

	for _, wantID := range []int64{1, 42, 1<<53 + 1, 1<<63 - 1} {
		t.Run(strconv.FormatInt(wantID, 10), func(t *testing.T) {
			token, err := manager.Generate(wantID)
			if err != nil {
				t.Fatalf("Generate(): %v", err)
			}

			gotID, err := manager.Parse(token)
			if err != nil {
				t.Fatalf("Parse(): %v", err)
			}
			if gotID != wantID {
				t.Errorf("Parse() userID = %d, want %d", gotID, wantID)
			}
		})
	}
}

func TestTokenManagerGenerateInvalidID(t *testing.T) {
	manager, _ := newTestTokenManager(t)

	for _, userID := range []int64{0, -1} {
		token, err := manager.Generate(userID)
		if err == nil {
			t.Errorf("Generate(%d) must return an error", userID)
		}
		if token != "" {
			t.Errorf("Generate(%d) must return an empty token", userID)
		}
	}
}

func TestTokenManagerGenerateClaims(t *testing.T) {
	manager, secret := newTestTokenManager(t)
	before := time.Now().UTC().Truncate(time.Second)

	signed, err := manager.Generate(42)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	after := time.Now().UTC()

	var claims jwt.RegisteredClaims
	_, err = jwt.ParseWithClaims(
		signed,
		&claims,
		func(_ *jwt.Token) (any, error) {
			return secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		t.Fatalf("Read generated claims: %v", err)
	}

	if claims.Subject != "42" || claims.Issuer != "task-manager" {
		t.Errorf("Unexpected subject or issuer: %+v", claims)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != "task-manager-api" {
		t.Errorf("Unexpected audience: %v", claims.Audience)
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatal("Generated token must contain iat and exp")
	}
	if claims.IssuedAt.Time.Before(before) || claims.IssuedAt.Time.After(after) {
		t.Errorf("Unexpected issued-at time: %v", claims.IssuedAt.Time)
	}

	lifetime := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if lifetime != 15*time.Minute {
		t.Errorf("Token lifetime = %v, want 15m", lifetime)
	}
}

func TestTokenManagerCopiesSecret(t *testing.T) {
	manager, originalSecret := newTestTokenManager(t)

	token, err := manager.Generate(42)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}

	originalSecret[0] ^= 1

	userID, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("Changing the original slice changed the manager's key: %v", err)
	}
	if userID != 42 {
		t.Errorf("Parse() userID = %d, want 42", userID)
	}
}

func TestTokenManagerParseInvalidTokens(t *testing.T) {
	manager, secret := newTestTokenManager(t)

	for _, input := range []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"malformed", "not-a-token"},
		{"invalid encoding", "a.b.c"},
	} {
		t.Run(input.name, func(t *testing.T) {
			assertInvalidToken(t, manager, input.token)
		})
	}

	tests := []struct {
		name   string
		change func(*jwt.RegisteredClaims)
		method jwt.SigningMethod
		key    any
	}{
		{
			name: "forged signature",
			key:  bytes.Repeat([]byte{2}, 64),
		},
		{
			name:   "unsupported algorithm",
			method: jwt.SigningMethodHS384,
		},
		{
			name:   "unsigned token",
			method: jwt.SigningMethodNone,
			key:    jwt.UnsafeAllowNoneSignatureType,
		},
		{
			name: "expired",
			change: func(c *jwt.RegisteredClaims) {
				c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
			},
		},
		{
			name:   "missing expiration",
			change: func(c *jwt.RegisteredClaims) { c.ExpiresAt = nil },
		},
		{
			name:   "wrong issuer",
			change: func(c *jwt.RegisteredClaims) { c.Issuer = "other-service" },
		},
		{
			name:   "missing issuer",
			change: func(c *jwt.RegisteredClaims) { c.Issuer = "" },
		},
		{
			name: "wrong audience",
			change: func(c *jwt.RegisteredClaims) {
				c.Audience = jwt.ClaimStrings{"other-api"}
			},
		},
		{
			name:   "missing audience",
			change: func(c *jwt.RegisteredClaims) { c.Audience = nil },
		},
		{
			name: "future issued-at",
			change: func(c *jwt.RegisteredClaims) {
				c.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
			},
		},
		{
			name:   "missing subject",
			change: func(c *jwt.RegisteredClaims) { c.Subject = "" },
		},
		{
			name:   "non-numeric subject",
			change: func(c *jwt.RegisteredClaims) { c.Subject = "abc" },
		},
		{
			name:   "zero subject",
			change: func(c *jwt.RegisteredClaims) { c.Subject = "0" },
		},
		{
			name:   "negative subject",
			change: func(c *jwt.RegisteredClaims) { c.Subject = "-1" },
		},
		{
			name:   "overflowing subject",
			change: func(c *jwt.RegisteredClaims) { c.Subject = "9223372036854775808" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := validTestClaims()
			if tt.change != nil {
				tt.change(&claims)
			}

			method := tt.method
			if method == nil {
				method = jwt.SigningMethodHS256
			}
			key := tt.key
			if key == nil {
				key = secret
			}

			signed, err := jwt.NewWithClaims(method, claims).SignedString(key)
			if err != nil {
				t.Fatalf("Create test token: %v", err)
			}

			assertInvalidToken(t, manager, signed)
		})
	}
}
