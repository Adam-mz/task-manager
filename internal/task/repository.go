package task

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("task not found")

type Repository interface {
	Create(ctx context.Context, task Task) (Task, error)
	GetByID(ctx context.Context, userID, id int64) (Task, error)
	List(ctx context.Context, userID int64, options ListOptions) ([]Task, error)
	Update(ctx context.Context, task Task) (Task, error)
	Delete(ctx context.Context, userID, id int64) error
}

var _ Repository = (*MemoryRepository)(nil)
