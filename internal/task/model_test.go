package task

import (
	"testing"
)

func TestTaskStatusIsValid(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   bool
	}{
		{
			name:   "статус todo валиден",
			status: StatusTodo,
			want:   true,
		},
		{
			name:   "статус in_progress валиден",
			status: StatusInProgress,
			want:   true,
		},
		{
			name:   "статус done валиден",
			status: StatusDone,
			want:   true,
		},
		{
			name:   "пустой статус невалиден",
			status: TaskStatus(""),
			want:   false,
		},
		{
			name:   "неизвестный статус невалиден",
			status: TaskStatus("unknown"),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsValid()
			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}

}
