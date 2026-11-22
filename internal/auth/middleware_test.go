package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func middlewareTestManager(t *testing.T) (*TokenManager, []byte) {
	t.Helper()
	// Deterministic key for tests only.
	secret := bytes.Repeat([]byte{1}, 32)
	manager, err := NewTokenManager(secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}
	return manager, secret
}

func TestTokenMiddlewareRejectsInvalidRequests(t *testing.T) {
	manager, secret := middlewareTestManager(t)
	validToken, err := manager.Generate(42)
	if err != nil {
		t.Fatalf("Generate token: %v", err)
	}

	otherManager, err := NewTokenManager(bytes.Repeat([]byte{2}, 32), 15*time.Minute)
	if err != nil {
		t.Fatalf("Create other token manager: %v", err)
	}
	wrongKeyToken, err := otherManager.Generate(42)
	if err != nil {
		t.Fatalf("Generate token signed with other key: %v", err)
	}

	now := time.Now().UTC()
	expiredToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "42",
		Issuer:    tokenIssuer,
		Audience:  jwt.ClaimStrings{tokenAudience},
		IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
	}).SignedString(secret)
	if err != nil {
		t.Fatalf("Sign expired token: %v", err)
	}

	tests := []struct {
		name   string
		header string
	}{
		{name: "missing header"},
		{name: "whitespace only", header: "   "},
		{name: "missing token", header: "Bearer"},
		{name: "token without scheme", header: validToken},
		{name: "wrong scheme with valid token", header: "Basic " + validToken},
		{name: "extra field", header: "Bearer " + validToken + " extra"},
		{name: "malformed token", header: "Bearer not-a-jwt"},
		{name: "wrong signing key", header: "Bearer " + wrongKeyToken},
		{name: "expired token", header: "Bearer " + expiredToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/tasks", nil)
			if tt.header != "" {
				request.Header.Set("Authorization", tt.header)
			}
			recorder := httptest.NewRecorder()
			manager.Middleware(next).ServeHTTP(recorder, request)

			if called {
				t.Error("Next handler called for an unauthorized request")
			}
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("Status = %d, want 401", recorder.Code)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", got)
			}
			var response map[string]any
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode error response: %v", err)
			}
			if len(response) != 1 || response["error"] != "unauthorized" {
				t.Errorf("Error response = %v, want only error=unauthorized", response)
			}
		})
	}
}

func TestTokenMiddlewarePassesAuthenticatedRequest(t *testing.T) {
	manager, _ := middlewareTestManager(t)
	token, err := manager.Generate(42)
	if err != nil {
		t.Fatalf("Generate token: %v", err)
	}

	for _, scheme := range []string{"Bearer", "bearer", "bEaReR"} {
		t.Run(scheme, func(t *testing.T) {
			type originalContextKey struct{}
			parent := context.WithValue(context.Background(), originalContextKey{}, "preserved")
			request := httptest.NewRequest(http.MethodGet, "/tasks", nil).WithContext(parent)
			request.Header.Set("Authorization", scheme+" "+token)

			calls := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				id, ok := UserIDFromContext(r.Context())
				if !ok || id != 42 {
					t.Errorf("Context user ID = %d, ok = %v; want 42, true", id, ok)
				}
				if got := r.Context().Value(originalContextKey{}); got != "preserved" {
					t.Errorf("Original context value = %v, want preserved", got)
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte("next handler"))
			})

			recorder := httptest.NewRecorder()
			manager.Middleware(next).ServeHTTP(recorder, request)
			if calls != 1 {
				t.Errorf("Next handler calls = %d, want 1", calls)
			}
			if recorder.Code != http.StatusAccepted || recorder.Body.String() != "next handler" {
				t.Errorf("Next handler response = %d %q, want 202 and next handler", recorder.Code, recorder.Body.String())
			}
			if _, ok := UserIDFromContext(request.Context()); ok {
				t.Error("Middleware changed the original request context")
			}
		})
	}
}

func TestUserIDFromContextValues(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		ok   bool
	}{
		{name: "missing", ctx: context.Background()},
		{name: "wrong type", ctx: context.WithValue(context.Background(), userIDKey{}, "42")},
		{name: "zero", ctx: context.WithValue(context.Background(), userIDKey{}, int64(0))},
		{name: "negative", ctx: context.WithValue(context.Background(), userIDKey{}, int64(-1))},
		{name: "positive", ctx: context.WithValue(context.Background(), userIDKey{}, int64(42)), ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := UserIDFromContext(tt.ctx)
			if ok != tt.ok {
				t.Errorf("UserIDFromContext ok = %v, want %v", ok, tt.ok)
			}
			if ok && id != 42 {
				t.Errorf("UserIDFromContext ID = %d, want 42", id)
			}
		})
	}
}
