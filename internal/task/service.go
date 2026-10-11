package task

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrTitleRequired     = errors.New("task title is required")
	ErrInvalidStatus     = errors.New("invalid task status")
	ErrInvalidPagination = errors.New("limit must be between 1 and 100; offset must be non-negative")
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
	}
}

func (s *Service) Create(ctx context.Context, title string, description string) (Task, error) {
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return Task{}, ErrTitleRequired
	}
	cleanDescription := strings.TrimSpace(description)
	task := Task{
		Title:       cleanTitle,
		Description: cleanDescription,
		Status:      StatusTodo,
	}
	return s.repository.Create(ctx, task)
}

func (s *Service) GetByID(ctx context.Context, id int64) (Task, error) {
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	return s.repository.GetByID(ctx, id)
}
func (s *Service) List(ctx context.Context, options ListOptions) ([]Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.Status != "" && !options.Status.IsValid() {
		return nil, ErrInvalidStatus
	}
	if options.Limit < 1 || options.Limit > 100 || options.Offset < 0 {
		return nil, ErrInvalidPagination
	}
	return s.repository.List(ctx, options)
}

func (s *Service) Update(ctx context.Context, id int64, title string, description string, status TaskStatus) (Task, error) {
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return Task{}, ErrTitleRequired
	}
	cleanDescription := strings.TrimSpace(description)
	if !status.IsValid() {
		return Task{}, ErrInvalidStatus
	}
	task := Task{
		ID:          id,
		Title:       cleanTitle,
		Description: cleanDescription,
		Status:      status,
	}
	return s.repository.Update(ctx, task)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.repository.Delete(ctx, id)
}
