package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"task-manager/internal/auth"
)

const (
	authFlowEmail     = "user@example.com"
	authFlowPassword  = "A long demo password 2026!"
	authFlowValidBody = `{"email":"user@example.com","password":"A long demo password 2026!"}`
)

type authFlowRepository struct {
	create     func(context.Context, User) (User, error)
	getByEmail func(context.Context, string) (User, error)
}

func (r *authFlowRepository) Create(ctx context.Context, input User) (User, error) {
	if r.create == nil {
		return User{}, errors.New("unexpected Create call")
	}
	return r.create(ctx, input)
}

func (r *authFlowRepository) GetByEmail(ctx context.Context, email string) (User, error) {
	if r.getByEmail == nil {
		return User{}, ErrNotFound
	}
	return r.getByEmail(ctx, email)
}

func authFlowHash(t *testing.T) string {
	t.Helper()
	hash, err := hashPassword(authFlowPassword)
	if err != nil {
		t.Fatalf("Hash test password: %v", err)
	}
	return hash
}

func authFlowHandler(t *testing.T, repository Repository) (http.Handler, *auth.TokenManager) {
	t.Helper()
	// Fixed key for tests only; production configuration is not used here.
	tokens, err := auth.NewTokenManager(bytes.Repeat([]byte{1}, 32), 15*time.Minute)
	if err != nil {
		t.Fatalf("Create token manager: %v", err)
	}
	return NewHTTPHandler(NewService(repository), tokens).Routes(), tokens
}

func authFlowPost(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if path == "/auth/login" && recorder.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Login Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
	}
	return recorder
}

func authFlowResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var response map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	return response
}

func authFlowError(t *testing.T, recorder *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("Status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}
	response := authFlowResponse(t, recorder)
	if len(response) != 1 || response["error"] != message {
		t.Errorf("Error response = %v, want only error=%q", response, message)
	}
}

func TestAuthFlowServiceAuthenticate(t *testing.T) {
	hash := authFlowHash(t)
	storageErr := errors.New("database unavailable")
	stored := User{ID: 42, Email: authFlowEmail, PasswordHash: hash}
	tests := []struct {
		name       string
		email      string
		password   string
		lookupErr  error
		brokenHash bool
		wantErr    error
	}{
		{name: "normalized email", email: " USER@EXAMPLE.COM ", password: authFlowPassword},
		{name: "wrong password", email: authFlowEmail, password: "wrong password", wantErr: ErrInvalidCredentials},
		{name: "unknown email", email: authFlowEmail, password: authFlowPassword, lookupErr: ErrNotFound, wantErr: ErrInvalidCredentials},
		{name: "repository failure", email: authFlowEmail, password: authFlowPassword, lookupErr: storageErr, wantErr: storageErr},
		{name: "corrupted stored hash", email: authFlowEmail, password: authFlowPassword, brokenHash: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &authFlowRepository{
				getByEmail: func(_ context.Context, email string) (User, error) {
					if email != authFlowEmail {
						t.Errorf("Repository email = %q, want %q", email, authFlowEmail)
					}
					if tt.lookupErr != nil {
						return User{}, tt.lookupErr
					}
					found := stored
					if tt.brokenHash {
						found.PasswordHash = "broken-hash"
					}
					return found, nil
				},
			}
			got, err := NewService(repository).Authenticate(context.Background(), tt.email, tt.password)
			if tt.brokenHash {
				if err == nil || errors.Is(err, ErrInvalidCredentials) {
					t.Fatalf("Corrupted hash error = %v, want an internal error", err)
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Authenticate error = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				if got != (User{}) {
					t.Error("Authenticate returned user data on failure")
				}
				return
			}
			if got != stored {
				t.Errorf("Authenticate returned unexpected user: ID=%d, email=%q", got.ID, got.Email)
			}
		})
	}
}

func TestAuthFlowServiceAuthenticateCancellation(t *testing.T) {
	hash := authFlowHash(t)
	for _, beforeLookup := range []bool{true, false} {
		name := "canceled during lookup"
		if beforeLookup {
			name = "already canceled"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if beforeLookup {
				cancel()
			}
			called := false
			repository := &authFlowRepository{
				getByEmail: func(context.Context, string) (User, error) {
					called = true
					cancel()
					return User{ID: 42, Email: authFlowEmail, PasswordHash: hash}, nil
				},
			}
			got, err := NewService(repository).Authenticate(ctx, authFlowEmail, authFlowPassword)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Authenticate error = %v, want context.Canceled", err)
			}
			if got != (User{}) {
				t.Error("Authenticate returned user data after cancellation")
			}
			if beforeLookup && called {
				t.Error("Repository called with an already canceled context")
			}
			if !beforeLookup && !called {
				t.Error("Repository was not called")
			}
		})
	}
}

func TestAuthFlowRegisterSuccess(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repository := &authFlowRepository{
		create: func(_ context.Context, input User) (User, error) {
			input.ID = 42
			input.CreatedAt = createdAt
			return input, nil
		},
	}
	handler, _ := authFlowHandler(t, repository)
	recorder := authFlowPost(t, handler, "/auth/register", authFlowValidBody)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("Register status = %d, want 201; body = %s", recorder.Code, recorder.Body.String())
	}
	response := authFlowResponse(t, recorder)
	if len(response) != 3 || response["id"] != float64(42) ||
		response["email"] != authFlowEmail || response["created_at"] != createdAt.Format(time.RFC3339Nano) {
		t.Errorf("Register response must contain only id, email and created_at; got keys/values: %v", response)
	}
}

func TestAuthFlowLoginSuccess(t *testing.T) {
	stored := User{ID: 42, Email: authFlowEmail, PasswordHash: authFlowHash(t)}
	repository := &authFlowRepository{
		getByEmail: func(context.Context, string) (User, error) {
			return stored, nil
		},
	}
	handler, tokens := authFlowHandler(t, repository)
	recorder := authFlowPost(t, handler, "/auth/login", authFlowValidBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("Login status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}
	response := authFlowResponse(t, recorder)
	if len(response) != 2 || response["token_type"] != "Bearer" {
		t.Fatal("Login response must contain only access_token and token_type=Bearer")
	}
	accessToken, ok := response["access_token"].(string)
	if !ok || accessToken == "" {
		t.Fatal("Login response has no nonempty access_token")
	}
	userID, err := tokens.Parse(accessToken)
	if err != nil || userID != stored.ID {
		t.Errorf("Login token user ID = %d, error = %v; want ID %d", userID, err, stored.ID)
	}
}

func TestAuthFlowInvalidBodies(t *testing.T) {
	repository := &authFlowRepository{
		create: func(context.Context, User) (User, error) {
			t.Error("Create called for an invalid request body")
			return User{}, errors.New("invalid body reached repository")
		},
		getByEmail: func(context.Context, string) (User, error) {
			t.Error("GetByEmail called for an invalid request body")
			return User{}, errors.New("invalid body reached repository")
		},
	}
	handler, _ := authFlowHandler(t, repository)
	tests := []struct {
		name    string
		body    string
		status  int
		message string
	}{
		{name: "empty", body: "", status: http.StatusBadRequest, message: "invalid request body"},
		{name: "broken JSON", body: "{", status: http.StatusBadRequest, message: "invalid request body"},
		{name: "unknown field", body: `{"email":"user@example.com","unexpected":true}`, status: http.StatusBadRequest, message: "invalid request body"},
		{name: "wrong field type", body: `{"email":42}`, status: http.StatusBadRequest, message: "invalid request body"},
		{name: "two JSON values", body: authFlowValidBody + " {}", status: http.StatusBadRequest, message: "invalid request body"},
		{name: "trailing garbage", body: authFlowValidBody + " garbage", status: http.StatusBadRequest, message: "invalid request body"},
		{name: "oversized body", body: `{"password":"` + strings.Repeat("x", 1<<20) + `"}`, status: http.StatusRequestEntityTooLarge, message: "request body too large"},
		{name: "oversized trailing data", body: authFlowValidBody + strings.Repeat(" ", 1<<20), status: http.StatusRequestEntityTooLarge, message: "request body too large"},
	}
	for _, path := range []string{"/auth/register", "/auth/login"} {
		t.Run(path, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					recorder := authFlowPost(t, handler, path, tt.body)
					authFlowError(t, recorder, tt.status, tt.message)
				})
			}
		})
	}
}

func TestAuthFlowRegisterErrors(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		createErr error
		status    int
		message   string
	}{
		{name: "invalid email", body: `{"email":"invalid","password":"A long demo password 2026!"}`, status: http.StatusBadRequest, message: ErrInvalidEmail.Error()},
		{name: "short password", body: `{"email":"user@example.com","password":"short"}`, status: http.StatusBadRequest, message: ErrInvalidPassword.Error()},
		{name: "duplicate email", body: authFlowValidBody, createErr: ErrEmailAlreadyExists, status: http.StatusConflict, message: ErrEmailAlreadyExists.Error()},
		{name: "repository failure", body: authFlowValidBody, createErr: errors.New("private database details"), status: http.StatusInternalServerError, message: "internal server error"},
		{name: "timeout", body: authFlowValidBody, createErr: context.DeadlineExceeded, status: http.StatusServiceUnavailable, message: "request timed out"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &authFlowRepository{
				create: func(context.Context, User) (User, error) {
					return User{}, tt.createErr
				},
			}
			handler, _ := authFlowHandler(t, repository)
			recorder := authFlowPost(t, handler, "/auth/register", tt.body)
			authFlowError(t, recorder, tt.status, tt.message)
		})
	}
}

func TestAuthFlowLoginErrors(t *testing.T) {
	hash := authFlowHash(t)
	tests := []struct {
		name       string
		body       string
		lookupErr  error
		brokenHash bool
		status     int
		message    string
	}{
		{name: "wrong password", body: `{"email":"user@example.com","password":"wrong password"}`, status: http.StatusUnauthorized, message: ErrInvalidCredentials.Error()},
		{name: "unknown email", body: authFlowValidBody, lookupErr: ErrNotFound, status: http.StatusUnauthorized, message: ErrInvalidCredentials.Error()},
		{name: "repository failure", body: authFlowValidBody, lookupErr: errors.New("private database details"), status: http.StatusInternalServerError, message: "internal server error"},
		{name: "corrupted stored hash", body: authFlowValidBody, brokenHash: true, status: http.StatusInternalServerError, message: "internal server error"},
		{name: "timeout", body: authFlowValidBody, lookupErr: context.DeadlineExceeded, status: http.StatusServiceUnavailable, message: "request timed out"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &authFlowRepository{
				getByEmail: func(context.Context, string) (User, error) {
					if tt.lookupErr != nil {
						return User{}, tt.lookupErr
					}
					stored := User{ID: 42, Email: authFlowEmail, PasswordHash: hash}
					if tt.brokenHash {
						stored.PasswordHash = "broken-hash"
					}
					return stored, nil
				},
			}
			handler, _ := authFlowHandler(t, repository)
			recorder := authFlowPost(t, handler, "/auth/login", tt.body)
			authFlowError(t, recorder, tt.status, tt.message)
		})
	}
}
