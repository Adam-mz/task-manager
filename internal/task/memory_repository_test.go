package task

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()

	first, err := repository.Create(ctx, Task{
		Title:       "First task",
		Description: "First description",
		Status:      StatusTodo,
	})
	if err != nil {
		t.Fatalf("Create() first task error = %v", err)
	}
	if first.ID != 1 {
		t.Errorf("Create() first task ID = %d, want 1", first.ID)
	}
	if first.CreatedAt.IsZero() {
		t.Error("Create() first task CreatedAt is zero")
	}
	if !first.CreatedAt.Equal(first.UpdatedAt) {
		t.Errorf("Create() first task CreatedAt = %v, UpdatedAt = %v, want equal", first.CreatedAt, first.UpdatedAt)
	}

	second, err := repository.Create(ctx, Task{
		Title:       "Second task",
		Description: "Second description",
		Status:      StatusTodo,
	})
	if err != nil {
		t.Fatalf("Create() second task error = %v", err)
	}
	if second.ID != 2 {
		t.Errorf("Create() second task ID = %d, want 2", second.ID)
	}

	got, err := repository.GetByID(ctx, 1)
	if err != nil {
		t.Fatalf("GetByID(1) error = %v", err)
	}
	if got.Title != first.Title {
		t.Errorf("GetByID(1) Title = %q, want %q", got.Title, first.Title)
	}

	tasks, err := repository.List(ctx, ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("List() length = %d, want 2", len(tasks))
	}
	if tasks[0].ID != 1 || tasks[1].ID != 2 {
		t.Errorf("List() IDs = [%d, %d], want [1, 2]", tasks[0].ID, tasks[1].ID)
	}

	updated, err := repository.Update(ctx, Task{
		ID:          first.ID,
		Title:       "Updated task",
		Description: "Updated description",
		Status:      StatusInProgress,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Title != "Updated task" {
		t.Errorf("Update() Title = %q, want %q", updated.Title, "Updated task")
	}
	if updated.Description != "Updated description" {
		t.Errorf("Update() Description = %q, want %q", updated.Description, "Updated description")
	}
	if updated.Status != StatusInProgress {
		t.Errorf("Update() Status = %q, want %q", updated.Status, StatusInProgress)
	}
	if !updated.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("Update() CreatedAt = %v, want %v", updated.CreatedAt, first.CreatedAt)
	}
	if updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Errorf("Update() UpdatedAt = %v, want not before CreatedAt = %v", updated.UpdatedAt, updated.CreatedAt)
	}

	if err := repository.Delete(ctx, first.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = repository.GetByID(ctx, first.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryRepositoryListPagination(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	createdIDs := make([]int64, 0, 5)
	for _, title := range []string{"First task", "Second task", "Third task", "Fourth task", "Fifth task"} {
		created, err := repository.Create(ctx, Task{Title: title, Status: StatusTodo})
		if err != nil {
			t.Fatalf("Create() %q error = %v", title, err)
		}
		createdIDs = append(createdIDs, created.ID)
	}

	tests := []struct {
		name    string
		options ListOptions
		wantIDs []int64
	}{
		{name: "first page", options: ListOptions{Limit: 2, Offset: 0}, wantIDs: createdIDs[:2]},
		{name: "second page", options: ListOptions{Limit: 2, Offset: 2}, wantIDs: createdIDs[2:4]},
		{name: "last partial page", options: ListOptions{Limit: 2, Offset: 4}, wantIDs: createdIDs[4:]},
		{name: "offset at end", options: ListOptions{Limit: 2, Offset: 5}},
		{name: "offset beyond end", options: ListOptions{Limit: 2, Offset: 100}},
		{name: "limit exceeds task count", options: ListOptions{Limit: 10, Offset: 0}, wantIDs: createdIDs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := repository.List(ctx, tt.options)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if tasks == nil {
				t.Fatal("List() returned nil, want a non-nil slice")
			}
			if len(tasks) != len(tt.wantIDs) {
				t.Fatalf("List() length = %d, want %d", len(tasks), len(tt.wantIDs))
			}
			for i, wantID := range tt.wantIDs {
				if tasks[i].ID != wantID {
					t.Errorf("List() task %d ID = %d, want %d", i, tasks[i].ID, wantID)
				}
			}
		})
	}
}

func TestMemoryRepositoryListEmpty(t *testing.T) {
	repository := NewMemoryRepository()
	tasks, err := repository.List(context.Background(), ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if tasks == nil {
		t.Fatal("List() returned nil, want a non-nil slice")
	}
	if len(tasks) != 0 {
		t.Fatalf("List() length = %d, want 0", len(tasks))
	}
}
