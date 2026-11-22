package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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

func createPostgresTestUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var userID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id
	`, fmt.Sprintf("task-owner-%d@example.com", time.Now().UnixNano()), "test-hash").Scan(&userID)
	if err != nil {
		t.Fatalf("Create task owner: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("Cleanup task owner: %v", err)
		}
	})
	return userID
}

func createPostgresTestTask(t *testing.T, repository *PostgresRepository, input Task) Task {
	t.Helper()
	if input.UserID == 0 {
		input.UserID = createPostgresTestUser(t, repository.pool)
	}
	created, err := repository.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := repository.Delete(ctx, created.UserID, created.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			t.Errorf("Cleanup task: %v", err)
		}
	})
	if created.UserID != input.UserID {
		t.Errorf("Created UserID = %d, want %d", created.UserID, input.UserID)
	}
	return created
}

func assertPostgresTaskEqual(t *testing.T, got, want Task) {
	t.Helper()
	if got.ID != want.ID || got.UserID != want.UserID || got.Title != want.Title || got.Description != want.Description || got.Status != want.Status {
		t.Errorf("Task fields: got %+v, want %+v", got, want)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("Task dates: got (%v, %v), want (%v, %v)", got.CreatedAt, got.UpdatedAt, want.CreatedAt, want.UpdatedAt)
	}
}

func TestPostgresRepositoryCreateAndGet(t *testing.T) {
	repository := NewPostgresRepository(openTestPostgres(t))
	input := Task{Title: "Learn PostgreSQL", Description: "Repository integration test", Status: StatusTodo}
	created := createPostgresTestTask(t, repository, input)
	if created.ID <= 0 {
		t.Errorf("Expected positive ID, got %d", created.ID)
	}
	if created.Title != input.Title || created.Description != input.Description || created.Status != input.Status {
		t.Errorf("Created fields: got %+v, input %+v", created, input)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("Created task dates must be nonzero")
	}
	var storedUserID int64
	if err := repository.pool.QueryRow(t.Context(), "SELECT user_id FROM tasks WHERE id = $1", created.ID).Scan(&storedUserID); err != nil {
		t.Fatalf("Read stored task owner: %v", err)
	}
	if storedUserID != created.UserID {
		t.Errorf("Stored UserID = %d, want %d", storedUserID, created.UserID)
	}
	got, err := repository.GetByID(context.Background(), created.UserID, created.ID)
	if err != nil {
		t.Fatalf("Get task: %v", err)
	}
	assertPostgresTaskEqual(t, got, created)
	if got.UserID != created.UserID {
		t.Errorf("GetByID() UserID = %d, want %d", got.UserID, created.UserID)
	}
	otherUserID := createPostgresTestUser(t, repository.pool)
	if foreign, err := repository.GetByID(t.Context(), otherUserID, created.ID); !errors.Is(err, ErrNotFound) || foreign != (Task{}) {
		t.Errorf("GetByID() for another user = (%+v, %v), want zero Task and ErrNotFound", foreign, err)
	}
}

func TestPostgresRepositoryUpdate(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)
	ctx := context.Background()
	created := createPostgresTestTask(t, repository, Task{Title: "Before update", Description: "Original description", Status: StatusTodo})
	if _, err := pool.Exec(ctx, "UPDATE tasks SET updated_at = created_at - INTERVAL '1 day' WHERE id = $1", created.ID); err != nil {
		t.Fatalf("Set original update date: %v", err)
	}
	original, err := repository.GetByID(ctx, created.UserID, created.ID)
	if err != nil {
		t.Fatalf("Get original task: %v", err)
	}
	input := original
	input.Title = "After update"
	input.Description = "Updated description"
	input.Status = StatusInProgress
	input.UserID = createPostgresTestUser(t, pool)
	if foreign, err := repository.Update(ctx, input); !errors.Is(err, ErrNotFound) || foreign != (Task{}) {
		t.Fatalf("Update() for another user = (%+v, %v), want zero Task and ErrNotFound", foreign, err)
	}
	stored, err := repository.GetByID(ctx, original.UserID, original.ID)
	if err != nil {
		t.Fatalf("Get task after foreign update: %v", err)
	}
	assertPostgresTaskEqual(t, stored, original)

	input.UserID = original.UserID
	updated, err := repository.Update(ctx, input)
	if err != nil {
		t.Fatalf("Update task: %v", err)
	}
	if updated.ID != original.ID || updated.UserID != original.UserID || updated.Title != input.Title || updated.Description != input.Description || updated.Status != input.Status {
		t.Errorf("Updated fields: got %+v, want %+v", updated, input)
	}
	if !updated.CreatedAt.Equal(original.CreatedAt) {
		t.Error("Update changed CreatedAt")
	}
	if !updated.UpdatedAt.After(original.UpdatedAt) {
		t.Errorf("UpdatedAt %v must be later than %v", updated.UpdatedAt, original.UpdatedAt)
	}
	got, err := repository.GetByID(ctx, created.UserID, updated.ID)
	if err != nil {
		t.Fatalf("Get updated task: %v", err)
	}
	assertPostgresTaskEqual(t, got, updated)
}

func TestPostgresRepositoryDelete(t *testing.T) {
	repository := NewPostgresRepository(openTestPostgres(t))
	ctx := context.Background()
	created := createPostgresTestTask(t, repository, Task{Title: "Delete me", Status: StatusTodo})
	otherUserID := createPostgresTestUser(t, repository.pool)
	if err := repository.Delete(ctx, otherUserID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete task for another user: got %v, want ErrNotFound", err)
	}
	stored, err := repository.GetByID(ctx, created.UserID, created.ID)
	if err != nil {
		t.Fatalf("Get task after foreign delete: %v", err)
	}
	assertPostgresTaskEqual(t, stored, created)

	if err := repository.Delete(ctx, created.UserID, created.ID); err != nil {
		t.Fatalf("Delete task: %v", err)
	}
	if _, err := repository.GetByID(ctx, created.UserID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get deleted task: got %v, want ErrNotFound", err)
	}
	if err := repository.Delete(ctx, created.UserID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete again: got %v, want ErrNotFound", err)
	}
}

func TestPostgresRepositoryNotFound(t *testing.T) {
	repository := NewPostgresRepository(openTestPostgres(t))
	ctx := context.Background()
	created := createPostgresTestTask(t, repository, Task{Title: "Missing task", Status: StatusTodo})
	if err := repository.Delete(ctx, created.UserID, created.ID); err != nil {
		t.Fatalf("Delete task: %v", err)
	}
	if _, err := repository.GetByID(ctx, created.UserID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing task: got %v, want ErrNotFound", err)
	}
	if _, err := repository.Update(ctx, created); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update missing task: got %v, want ErrNotFound", err)
	}
	if err := repository.Delete(ctx, created.UserID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing task: got %v, want ErrNotFound", err)
	}
}

func TestPostgresRepositoryList(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)
	userID := createPostgresTestUser(t, pool)
	first := createPostgresTestTask(t, repository, Task{UserID: userID, Title: "First list task", Description: "First description", Status: StatusTodo})
	second := createPostgresTestTask(t, repository, Task{UserID: userID, Title: "Second list task", Description: "Second description", Status: StatusInProgress})
	tasks, err := repository.List(t.Context(), userID, ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("List length = %d, want 2", len(tasks))
	}
	found := make(map[int64]Task)
	for i, task := range tasks {
		if i > 0 && tasks[i-1].ID >= task.ID {
			t.Errorf("List IDs must increase: %d followed by %d", tasks[i-1].ID, task.ID)
		}
		found[task.ID] = task
	}
	for _, want := range []Task{first, second} {
		got, ok := found[want.ID]
		if !ok {
			t.Errorf("Task %d missing from list", want.ID)
			continue
		}
		assertPostgresTaskEqual(t, got, want)
		if got.UserID != userID {
			t.Errorf("List UserID = %d, want %d", got.UserID, userID)
		}
	}
}

func TestPostgresRepositoryListStatusFilter(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)
	userID := createPostgresTestUser(t, pool)

	createPostgresTestTask(t, repository, Task{
		UserID: userID, Title: "A", Status: StatusTodo,
	})
	firstDone := createPostgresTestTask(t, repository, Task{
		UserID: userID, Title: "B", Status: StatusDone,
	})
	createPostgresTestTask(t, repository, Task{
		UserID: userID, Title: "C", Status: StatusInProgress,
	})
	secondDone := createPostgresTestTask(t, repository, Task{
		UserID: userID, Title: "D", Status: StatusDone,
	})

	tests := []struct {
		name    string
		options ListOptions
		want    []Task
	}{
		{
			name: "done tasks",
			options: ListOptions{
				Limit: 2, Status: StatusDone,
			},
			want: []Task{firstDone, secondDone},
		},
		{
			name: "filter before pagination",
			options: ListOptions{
				Limit: 1, Offset: 1, Status: StatusDone,
			},
			want: []Task{secondDone},
		},
		{
			name: "done tasks in descending order",
			options: ListOptions{
				Status: StatusDone, Order: "desc", Limit: 2,
			},
			want: []Task{secondDone, firstDone},
		},
		{
			name: "descending order before pagination",
			options: ListOptions{
				Status: StatusDone, Order: "desc", Limit: 1, Offset: 1,
			},
			want: []Task{firstDone},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := repository.List(t.Context(), userID, tt.options)
			if err != nil {
				t.Fatalf("List tasks: %v", err)
			}
			if len(tasks) != len(tt.want) {
				t.Fatalf("List length = %d, want %d", len(tasks), len(tt.want))
			}
			for i, want := range tt.want {
				assertPostgresTaskEqual(t, tasks[i], want)
			}
		})
	}
}

func TestPostgresRepositoryListOwnership(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)
	userA := createPostgresTestUser(t, pool)
	userB := createPostgresTestUser(t, pool)

	var legacyTaskID int64
	err := pool.QueryRow(t.Context(), `
		INSERT INTO tasks (title, status) VALUES ('Legacy task', 'done') RETURNING id
	`).Scan(&legacyTaskID)
	if err != nil {
		t.Fatalf("Create legacy task without owner: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM tasks WHERE id = $1 AND user_id IS NULL", legacyTaskID); err != nil {
			t.Errorf("Cleanup legacy task: %v", err)
		}
	})

	testRepositoryListOwnership(t, repository, userA, userB)
}

func TestPostgresRepositoryConstraints(t *testing.T) {
	repository := NewPostgresRepository(openTestPostgres(t))
	userID := createPostgresTestUser(t, repository.pool)
	cases := []struct {
		name       string
		input      Task
		constraint string
	}{
		{name: "Empty title", input: Task{Title: "   ", Status: StatusTodo}, constraint: "tasks_title_check"},
		{name: "Unknown status", input: Task{Title: "Valid title", Status: TaskStatus("unknown")}, constraint: "tasks_status_check"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.UserID = userID
			created, err := repository.Create(context.Background(), tc.input)
			if err == nil {
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					if err := repository.Delete(ctx, created.UserID, created.ID); err != nil && !errors.Is(err, ErrNotFound) {
						t.Errorf("Cleanup task: %v", err)
					}
				})
				t.Fatal("Create must fail for invalid task")
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Fatalf("Expected PgError, got %T: %v", err, err)
			}
			if pgErr.Code != "23514" || pgErr.ConstraintName != tc.constraint {
				t.Errorf("Expected code 23514 and constraint %q, got %q and %q", tc.constraint, pgErr.Code, pgErr.ConstraintName)
			}
		})
	}
}
