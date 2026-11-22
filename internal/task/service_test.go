package task

import (
	"context"
	"errors"
	"testing"
)

func TestServiceCreate(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	service := NewService(repository)

	created, err := service.Create(ctx, 42, "  Learn Go  ", "  Study service layer  ")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Title != "Learn Go" {
		t.Errorf("Create() Title = %q, want %q", created.Title, "Learn Go")
	}
	if created.Description != "Study service layer" {
		t.Errorf("Create() Description = %q, want %q", created.Description, "Study service layer")
	}
	if created.Status != StatusTodo {
		t.Errorf("Create() Status = %q, want %q", created.Status, StatusTodo)
	}
	if created.ID != 1 {
		t.Errorf("Create() ID = %d, want 1", created.ID)
	}
	if created.UserID != 42 {
		t.Errorf("Create() UserID = %d, want 42", created.UserID)
	}

	_, err = service.Create(ctx, 42, " ", "Invalid task")
	if !errors.Is(err, ErrTitleRequired) {
		t.Errorf("Create() with blank title error = %v, want ErrTitleRequired", err)
	}

	tasks, err := service.List(ctx, 42, ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("List() length = %d, want 1", len(tasks))
	}
}

func TestServiceCreateInvalidUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID int64
	}{
		{name: "zero", userID: 0},
		{name: "negative", userID: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := NewMemoryRepository()
			service := NewService(repository)

			created, err := service.Create(t.Context(), tt.userID, "Task", "Description")
			if !errors.Is(err, ErrInvalidUserID) {
				t.Fatalf("Create() error = %v, want ErrInvalidUserID", err)
			}
			if created != (Task{}) {
				t.Errorf("Create() task = %+v, want zero Task", created)
			}
			tasks, err := repository.List(t.Context(), tt.userID, ListOptions{Limit: 100})
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(tasks) != 0 {
				t.Errorf("Invalid user ID created %d tasks, want 0", len(tasks))
			}
		})
	}
}

func TestServiceUpdate(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	service := NewService(repository)

	created, err := service.Create(ctx, 42, "Initial task", "Initial description")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := service.Update(
		ctx,
		created.UserID,
		created.ID,
		"  Updated task  ",
		"  Updated description  ",
		StatusInProgress,
	)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("Update() ID = %d, want %d", updated.ID, created.ID)
	}
	if updated.UserID != created.UserID {
		t.Errorf("Update() UserID = %d, want %d", updated.UserID, created.UserID)
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
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("Update() CreatedAt = %v, want %v", updated.CreatedAt, created.CreatedAt)
	}
	if updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Errorf("Update() UpdatedAt = %v, want not before CreatedAt = %v", updated.UpdatedAt, updated.CreatedAt)
	}

	_, err = service.Update(ctx, created.UserID, created.ID, " ", "Invalid description", StatusTodo)
	if !errors.Is(err, ErrTitleRequired) {
		t.Errorf("Update() with blank title error = %v, want ErrTitleRequired", err)
	}

	_, err = service.Update(ctx, created.UserID, created.ID, "Valid title", "Invalid status", TaskStatus("unknown"))
	if !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("Update() with invalid status error = %v, want ErrInvalidStatus", err)
	}

	stored, err := service.GetByID(ctx, 42, created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.Title != "Updated task" {
		t.Errorf("GetByID() Title = %q, want %q", stored.Title, "Updated task")
	}
	if stored.Status != StatusInProgress {
		t.Errorf("GetByID() Status = %q, want %q", stored.Status, StatusInProgress)
	}
}

func TestServiceUpdateInvalidUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID int64
	}{
		{name: "zero", userID: 0},
		{name: "negative", userID: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil)
			task, err := service.Update(t.Context(), tt.userID, 1, "Task", "Description", StatusDone)
			if !errors.Is(err, ErrInvalidUserID) {
				t.Fatalf("Update() error = %v, want ErrInvalidUserID", err)
			}
			if task != (Task{}) {
				t.Errorf("Update() task = %+v, want zero Task", task)
			}
		})
	}
}

func TestServiceGetListDelete(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	service := NewService(repository)

	first, err := service.Create(ctx, 42, "First task", "First description")
	if err != nil {
		t.Fatalf("Create() first task error = %v", err)
	}
	second, err := service.Create(ctx, 42, "Second task", "Second description")
	if err != nil {
		t.Fatalf("Create() second task error = %v", err)
	}

	got, err := service.GetByID(ctx, 42, second.ID)
	if err != nil {
		t.Fatalf("GetByID() second task error = %v", err)
	}
	if got.ID != second.ID {
		t.Errorf("GetByID() ID = %d, want %d", got.ID, second.ID)
	}
	if got.Title != second.Title {
		t.Errorf("GetByID() Title = %q, want %q", got.Title, second.Title)
	}

	tasks, err := service.List(ctx, 42, ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("List() length = %d, want 2", len(tasks))
	}
	if tasks[0].ID != 1 || tasks[1].ID != 2 {
		t.Errorf("List() IDs = [%d, %d], want [1, 2]", tasks[0].ID, tasks[1].ID)
	}

	if err := service.Delete(ctx, first.UserID, first.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = service.GetByID(ctx, 42, first.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID() after Delete() error = %v, want ErrNotFound", err)
	}

	err = service.Delete(ctx, first.UserID, first.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete() deleted task error = %v, want ErrNotFound", err)
	}

	tasks, err = service.List(ctx, 42, ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List() after Delete() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("List() after Delete() length = %d, want 1", len(tasks))
	}
	if tasks[0].ID != second.ID {
		t.Errorf("List() remaining task ID = %d, want %d", tasks[0].ID, second.ID)
	}
}

func TestServiceDeleteInvalidUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID int64
	}{
		{name: "zero", userID: 0},
		{name: "negative", userID: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil)
			if err := service.Delete(t.Context(), tt.userID, 1); !errors.Is(err, ErrInvalidUserID) {
				t.Fatalf("Delete() error = %v, want ErrInvalidUserID", err)
			}
		})
	}
}

func TestServiceGetByIDInvalidUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID int64
	}{
		{name: "zero", userID: 0},
		{name: "negative", userID: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil)
			task, err := service.GetByID(t.Context(), tt.userID, 1)
			if !errors.Is(err, ErrInvalidUserID) {
				t.Fatalf("GetByID() error = %v, want ErrInvalidUserID", err)
			}
			if task != (Task{}) {
				t.Errorf("GetByID() task = %+v, want zero Task", task)
			}
		})
	}
}

func TestServiceListPagination(t *testing.T) {
	ctx := context.Background()
	service := NewService(NewMemoryRepository())
	for i := 0; i < 3; i++ {
		if _, err := service.Create(ctx, 42, "Task", ""); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	tests := []struct {
		name    string
		options ListOptions
		wantIDs []int64
		wantErr error
	}{
		{name: "minimum limit with offset", options: ListOptions{Limit: 1, Offset: 1}, wantIDs: []int64{2}},
		{name: "maximum limit", options: ListOptions{Limit: 100}, wantIDs: []int64{1, 2, 3}},
		{name: "zero limit", options: ListOptions{}, wantErr: ErrInvalidPagination},
		{name: "negative limit", options: ListOptions{Limit: -1}, wantErr: ErrInvalidPagination},
		{name: "excessive limit", options: ListOptions{Limit: 101}, wantErr: ErrInvalidPagination},
		{name: "negative offset", options: ListOptions{Limit: 1, Offset: -1}, wantErr: ErrInvalidPagination},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := service.List(ctx, 42, tt.options)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("List() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
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

func TestServiceListPaginationValidation(t *testing.T) {
	service := NewService(NewMemoryRepository())

	tests := []struct {
		name    string
		options ListOptions
		wantErr error
	}{
		{name: "zero limit", options: ListOptions{Limit: 0, Offset: 0}, wantErr: ErrInvalidPagination},
		{name: "negative limit", options: ListOptions{Limit: -1, Offset: 0}, wantErr: ErrInvalidPagination},
		{name: "excessive limit", options: ListOptions{Limit: 101, Offset: 0}, wantErr: ErrInvalidPagination},
		{name: "negative offset", options: ListOptions{Limit: 20, Offset: -1}, wantErr: ErrInvalidPagination},
		{name: "minimum limit", options: ListOptions{Limit: 1, Offset: 0}, wantErr: nil},
		{name: "maximum limit", options: ListOptions{Limit: 100, Offset: 0}, wantErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.List(context.Background(), 42, tt.options)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("List() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceListInvalidUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID int64
	}{
		{name: "zero", userID: 0},
		{name: "negative", userID: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(nil)
			tasks, err := service.List(t.Context(), tt.userID, ListOptions{Limit: 20})
			if !errors.Is(err, ErrInvalidUserID) {
				t.Fatalf("List() error = %v, want ErrInvalidUserID", err)
			}
			if tasks != nil {
				t.Errorf("List() tasks = %+v, want nil", tasks)
			}
		})
	}
}
