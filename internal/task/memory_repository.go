package task

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu     sync.RWMutex
	tasks  map[int64]Task
	nextID int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		tasks: make(map[int64]Task),
	}
}

func (r *MemoryRepository) Create(ctx context.Context, task Task) (Task, error) {
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	task.ID = r.nextID
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now
	r.tasks[r.nextID] = task
	return task, nil
}

func (r *MemoryRepository) GetByID(ctx context.Context, id int64) (Task, error) {
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}
	return task, nil
}
func (r *MemoryRepository) List(ctx context.Context, options ListOptions) ([]Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()

	tasks := make([]Task, 0, len(r.tasks))

	for _, value := range r.tasks {
		if options.Status != "" && value.Status != options.Status {
			continue
		}
		tasks = append(tasks, value)
	}
	r.mu.RUnlock()
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	if options.Offset >= len(tasks) {
		return []Task{}, nil
	}

	tasks = tasks[options.Offset:]
	if len(tasks) > options.Limit {
		tasks = tasks[:options.Limit]
	}
	return tasks, nil
}

func (r *MemoryRepository) Update(ctx context.Context, task Task) (Task, error) {
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	taskUpdate, ok := r.tasks[task.ID]
	if !ok {
		return Task{}, ErrNotFound
	}

	taskUpdate.UpdatedAt = time.Now().UTC()
	taskUpdate.Description = task.Description
	taskUpdate.Status = task.Status
	taskUpdate.Title = task.Title

	r.tasks[taskUpdate.ID] = taskUpdate

	return taskUpdate, nil
}

func (r *MemoryRepository) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tasks[id]; !ok {
		return ErrNotFound
	}
	delete(r.tasks, id)
	return nil
}
