package task

import (
	"context"
	"testing"
	"time"
)

func testRepositoryListOwnership(t *testing.T, repository Repository, userA, userB int64) {
	t.Helper()
	created := make([]Task, 0, 3)
	for _, input := range []Task{
		{UserID: userA, Title: "A1", Status: StatusDone},
		{UserID: userB, Title: "B1", Status: StatusDone},
		{UserID: userA, Title: "A2", Status: StatusDone},
	} {
		task, err := repository.Create(t.Context(), input)
		if err != nil {
			t.Fatalf("Create %s: %v", input.Title, err)
		}
		created = append(created, task)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := repository.Delete(ctx, task.UserID, task.ID); err != nil {
				t.Errorf("Cleanup task %d: %v", task.ID, err)
			}
		})
	}

	tests := []struct {
		name    string
		userID  int64
		options ListOptions
		want    []Task
	}{
		{
			name: "owner A without status", userID: userA,
			options: ListOptions{Limit: 100, Order: "asc"},
			want:    []Task{created[0], created[2]},
		},
		{
			name: "owner A with status", userID: userA,
			options: ListOptions{Limit: 100, Status: StatusDone, Order: "asc"},
			want:    []Task{created[0], created[2]},
		},
		{
			name: "owner before ascending pagination", userID: userA,
			options: ListOptions{Limit: 1, Offset: 1, Status: StatusDone, Order: "asc"},
			want:    []Task{created[2]},
		},
		{
			name: "owner before descending pagination", userID: userA,
			options: ListOptions{Limit: 1, Offset: 1, Status: StatusDone, Order: "desc"},
			want:    []Task{created[0]},
		},
		{
			name: "owner B", userID: userB,
			options: ListOptions{Limit: 100},
			want:    []Task{created[1]},
		},
		{
			name: "offset at end of owner's tasks", userID: userA,
			options: ListOptions{Limit: 1, Offset: 2, Status: StatusDone},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := repository.List(t.Context(), tt.userID, tt.options)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if tasks == nil {
				t.Fatal("List() returned nil, want a non-nil slice")
			}
			if len(tasks) != len(tt.want) {
				t.Fatalf("List() tasks = %+v, want %+v", tasks, tt.want)
			}
			for i, want := range tt.want {
				if got := tasks[i]; got.ID != want.ID || got.Title != want.Title || got.UserID != tt.userID || got.Status != want.Status {
					t.Errorf("List() task %d = %+v, want %+v", i, got, want)
				}
			}
		})
	}
}
