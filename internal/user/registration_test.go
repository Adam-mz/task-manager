package user

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type registrationRepositoryStub struct {
	createFn func(context.Context, User) (User, error)
}

func (r *registrationRepositoryStub) Create(ctx context.Context, input User) (User, error) {
	return r.createFn(ctx, input)
}

func (r *registrationRepositoryStub) GetByEmail(context.Context, string) (User, error) {
	return User{}, ErrNotFound
}

func TestPasswordHashing(t *testing.T) {
	password := "  Correct horse battery staple!  "

	first, err := hashPassword(password)
	if err != nil {
		t.Fatalf("First hash: %v", err)
	}

	second, err := hashPassword(password)
	if err != nil {
		t.Fatalf("Second hash: %v", err)
	}

	if !strings.HasPrefix(first, "$argon2id$") {
		t.Fatal("Expected an Argon2id hash")
	}
	if first == second {
		t.Fatal("Repeated hashing must produce different salted hashes")
	}

	tests := []struct {
		name      string
		password  string
		hash      string
		wantMatch bool
		wantErr   bool
	}{
		{"correct password", password, first, true, false},
		{"wrong password", "another password", first, false, false},
		{"spaces preserved", strings.TrimSpace(password), first, false, false},
		{"invalid hash", password, "broken-hash", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := checkPassword(tt.password, tt.hash)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkPassword() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if match != tt.wantMatch {
				t.Errorf("checkPassword() match = %v, want %v", match, tt.wantMatch)
			}
		})
	}
}

func TestServiceRegister(t *testing.T) {
	password := "  Correct horse battery staple!  "
	createdAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		email    string
		password string
		wantErr  error
	}{
		{"success", "  ADAM@EXAMPLE.COM  ", password, nil},
		{"minimum length", "adam@example.com", strings.Repeat("a", 15), nil},
		{"maximum unicode length", "adam@example.com", strings.Repeat("я", 128), nil},
		{"empty email", "", password, ErrInvalidEmail},
		{"invalid email", "not-an-email", password, ErrInvalidEmail},
		{"display name", "Adam <adam@example.com>", password, ErrInvalidEmail},
		{"empty password", "adam@example.com", "", ErrInvalidPassword},
		{"short password", "adam@example.com", strings.Repeat("a", 14), ErrInvalidPassword},
		{"short unicode password", "adam@example.com", strings.Repeat("я", 14), ErrInvalidPassword},
		{"long password", "adam@example.com", strings.Repeat("a", 129), ErrInvalidPassword},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			var saved User

			repository := &registrationRepositoryStub{
				createFn: func(ctx context.Context, input User) (User, error) {
					called = true
					saved = input
					input.ID = 42
					input.CreatedAt = createdAt
					return input, nil
				},
			}
			service := NewService(repository)

			got, err := service.Register(t.Context(), tt.email, tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Register() error = %v, want %v", err, tt.wantErr)
			}

			if tt.wantErr != nil {
				if called {
					t.Error("Invalid input reached the repository")
				}
				return
			}

			if !called {
				t.Fatal("Repository Create was not called")
			}
			if saved.Email != "adam@example.com" {
				t.Errorf("Saved email = %q, want adam@example.com", saved.Email)
			}

			match, err := checkPassword(tt.password, saved.PasswordHash)
			if err != nil || !match {
				t.Fatalf("Saved hash does not match the original password: %v", err)
			}

			if got.ID != 42 ||
				got.Email != saved.Email ||
				got.PasswordHash != saved.PasswordHash ||
				!got.CreatedAt.Equal(createdAt) {
				t.Errorf("Register() returned unexpected user: %+v", got)
			}
		})
	}
}

func TestServiceRegisterErrors(t *testing.T) {
	storageErr := errors.New("database unavailable")

	tests := []struct {
		name     string
		canceled bool
		repoErr  error
		wantErr  error
	}{
		{"canceled context", true, nil, context.Canceled},
		{"duplicate email", false, ErrEmailAlreadyExists, ErrEmailAlreadyExists},
		{"repository error", false, storageErr, storageErr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.canceled {
				cancel()
			}

			called := false
			repository := &registrationRepositoryStub{
				createFn: func(context.Context, User) (User, error) {
					called = true
					return User{}, tt.repoErr
				},
			}
			service := NewService(repository)

			_, err := service.Register(ctx, "adam@example.com", "a long test password")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Register() error = %v, want %v", err, tt.wantErr)
			}
			if called != !tt.canceled {
				t.Errorf("Repository called = %v, want %v", called, !tt.canceled)
			}
		})
	}
}
