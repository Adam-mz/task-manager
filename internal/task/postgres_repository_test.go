package task

import (
	"context"
	"errors"
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

func createPostgresTestTask(t *testing.T, repository *PostgresRepository, input Task) Task {
	t.Helper()
	created, err := repository.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := repository.Delete(ctx, created.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			t.Errorf("Cleanup task: %v", err)
		}
	})
	return created
}

func assertPostgresTaskEqual(t *testing.T, got, want Task) {
	t.Helper()
	if got.ID != want.ID || got.Title != want.Title || got.Description != want.Description || got.Status != want.Status {
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
	got, err := repository.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Get task: %v", err)
	}
	assertPostgresTaskEqual(t, got, created)
}

func TestPostgresRepositoryUpdate(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)
	ctx := context.Background()
	created := createPostgresTestTask(t, repository, Task{Title: "Before update", Description: "Original description", Status: StatusTodo})
	if _, err := pool.Exec(ctx, "UPDATE tasks SET updated_at = created_at - INTERVAL '1 day' WHERE id = $1", created.ID); err != nil {
		t.Fatalf("Set original update date: %v", err)
	}
	original, err := repository.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get original task: %v", err)
	}
	input := original
	input.Title = "After update"
	input.Description = "Updated description"
	input.Status = StatusInProgress
	updated, err := repository.Update(ctx, input)
	if err != nil {
		t.Fatalf("Update task: %v", err)
	}
	if updated.ID != original.ID || updated.Title != input.Title || updated.Description != input.Description || updated.Status != input.Status {
		t.Errorf("Updated fields: got %+v, want %+v", updated, input)
	}
	if !updated.CreatedAt.Equal(original.CreatedAt) {
		t.Error("Update changed CreatedAt")
	}
	if !updated.UpdatedAt.After(original.UpdatedAt) {
		t.Errorf("UpdatedAt %v must be later than %v", updated.UpdatedAt, original.UpdatedAt)
	}
	got, err := repository.GetByID(ctx, updated.ID)
	if err != nil {
		t.Fatalf("Get updated task: %v", err)
	}
	assertPostgresTaskEqual(t, got, updated)
}

func TestPostgresRepositoryDelete(t *testing.T) {
	repository := NewPostgresRepository(openTestPostgres(t))
	ctx := context.Background()
	created := createPostgresTestTask(t, repository, Task{Title: "Delete me", Status: StatusTodo})
	if err := repository.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete task: %v", err)
	}
	if _, err := repository.GetByID(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get deleted task: got %v, want ErrNotFound", err)
	}
	if err := repository.Delete(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete again: got %v, want ErrNotFound", err)
	}
}

func TestPostgresRepositoryNotFound(t *testing.T) {
	repository := NewPostgresRepository(openTestPostgres(t))
	ctx := context.Background()
	created := createPostgresTestTask(t, repository, Task{Title: "Missing task", Status: StatusTodo})
	if err := repository.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete task: %v", err)
	}
	if _, err := repository.GetByID(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing task: got %v, want ErrNotFound", err)
	}
	if _, err := repository.Update(ctx, created); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update missing task: got %v, want ErrNotFound", err)
	}
	if err := repository.Delete(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing task: got %v, want ErrNotFound", err)
	}
}

func TestPostgresRepositoryList(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)
	first := createPostgresTestTask(t, repository, Task{Title: "First list task", Description: "First description", Status: StatusTodo})
	second := createPostgresTestTask(t, repository, Task{Title: "Second list task", Description: "Second description", Status: StatusInProgress})
	var offset int
	if err := pool.QueryRow(t.Context(), "SELECT COUNT(*) FROM tasks WHERE id < $1", first.ID).Scan(&offset); err != nil {
		t.Fatalf("Find test page offset: %v", err)
	}
	tasks, err := repository.List(t.Context(), ListOptions{Limit: 2, Offset: offset})
	if err != nil {
		t.Fatalf("List tasks: %v", err)
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
	}
}

func TestPostgresRepositoryListStatusFilter(t *testing.T) {
	pool := openTestPostgres(t)
	repository := NewPostgresRepository(pool)
	ctx := t.Context()

	createPostgresTestTask(t, repository, Task{
		Title: "A", Status: StatusTodo,
	})
	firstDone := createPostgresTestTask(t, repository, Task{
		Title: "B", Status: StatusDone,
	})
	createPostgresTestTask(t, repository, Task{
		Title: "C", Status: StatusInProgress,
	})
	secondDone := createPostgresTestTask(t, repository, Task{
		Title: "D", Status: StatusDone,
	})

	var offset int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE status = $1 AND id < $2
	`, string(StatusDone), firstDone.ID).Scan(&offset)
	if err != nil {
		t.Fatalf("Find filtered page offset: %v", err)
	}

	tests := []struct {
		name    string
		options ListOptions
		want    []Task
	}{
		{
			name: "done tasks",
			options: ListOptions{
				Limit: 2, Offset: offset, Status: StatusDone,
			},
			want: []Task{firstDone, secondDone},
		},
		{
			name: "filter before pagination",
			options: ListOptions{
				Limit: 1, Offset: offset + 1, Status: StatusDone,
			},
			want: []Task{secondDone},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := repository.List(t.Context(), tt.options)
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

func TestPostgresRepositoryConstraints(t *testing.T) {
	repository := NewPostgresRepository(openTestPostgres(t))
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
			created, err := repository.Create(context.Background(), tc.input)
			if err == nil {
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					if err := repository.Delete(ctx, created.ID); err != nil && !errors.Is(err, ErrNotFound) {
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
