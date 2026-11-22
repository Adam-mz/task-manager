package user

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func openTestPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задана")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("Create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping PostgreSQL: %v", err)
	}
	var database string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil {
		t.Fatalf("Read database name: %v", err)
	}
	if database != "task_manager_test" {
		t.Fatalf("Expected database task_manager_test, got %q", database)
	}
	return pool
}

func TestPostgresRepositoryCreateAndGetByEmail(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)
	input := User{
		Email:        fmt.Sprintf("user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "test-hash",
	}

	created, err := repository.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if _, err := pool.Exec(ctx,
			"DELETE FROM users WHERE id = $1", created.ID,
		); err != nil {
			t.Errorf("Cleanup user: %v", err)
		}
	})

	if created.ID <= 0 {
		t.Errorf("Expected positive ID, got %d", created.ID)
	}
	if created.Email != input.Email || created.PasswordHash != input.PasswordHash {
		t.Errorf("Created fields: got %+v, input %+v", created, input)
	}
	if created.CreatedAt.IsZero() {
		t.Error("Created user date must be nonzero")
	}

	got, err := repository.GetByEmail(context.Background(), strings.ToUpper(input.Email))
	if err != nil {
		t.Fatalf("Get user by email: %v", err)
	}
	if got.ID != created.ID || got.Email != created.Email || got.PasswordHash != created.PasswordHash {
		t.Errorf("User fields: got %+v, want %+v", got, created)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("User CreatedAt: got %v, want %v", got.CreatedAt, created.CreatedAt)
	}
}

func TestPostgresRepositoryCreateDuplicateEmail(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)
	input := User{
		Email:        fmt.Sprintf("user-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "test-hash",
	}

	_, err := repository.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if _, err := pool.Exec(ctx,
			"DELETE FROM users WHERE lower(email) = lower($1)", input.Email,
		); err != nil {
			t.Errorf("Cleanup user: %v", err)
		}
	})

	_, err = repository.Create(context.Background(), input)
	if !errors.Is(err, ErrEmailAlreadyExists) {
		t.Errorf("Create duplicate user: got %v, want ErrEmailAlreadyExists", err)
	}

	uppercaseInput := input
	uppercaseInput.Email = strings.ToUpper(input.Email)
	_, err = repository.Create(context.Background(), uppercaseInput)
	if !errors.Is(err, ErrEmailAlreadyExists) {
		t.Errorf("Create duplicate user: got %v, want ErrEmailAlreadyExists", err)
	}
}

func TestPostgresRepositoryGetByEmailNotFound(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)

	email := fmt.Sprintf("missing-%d@example.com", time.Now().UnixNano())

	_, err := repository.GetByEmail(context.Background(), email)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByEmail(): got %v, want ErrNotFound", err)
	}
}
