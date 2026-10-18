package task

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{
		pool: pool,
	}
}

func (p *PostgresRepository) GetByID(ctx context.Context, id int64) (Task, error) {
	var task Task

	err := p.pool.QueryRow(
		ctx,
		`SELECT id, title, description, status, created_at, updated_at
	 FROM tasks
	 WHERE id = $1`,
		id,
	).Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrNotFound
		}
		return Task{}, fmt.Errorf("get task by id: %w", err)
	}

	return task, nil
}

func (p *PostgresRepository) List(ctx context.Context, options ListOptions) ([]Task, error) {
	direction := "ASC"
	if options.Order == "desc" {
		direction = "DESC"
	}

	rows, err := p.pool.Query(ctx, `
	SELECT id, title, description, status, created_at, updated_at
	FROM tasks
	WHERE ($3::text = '' OR status = $3)
	ORDER BY id `+direction+` LIMIT $1 OFFSET $2
	`, options.Limit, options.Offset, string(options.Status))

	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		var task Task
		err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.CreatedAt, &task.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}

	return tasks, nil
}

func (p *PostgresRepository) Create(ctx context.Context, task Task) (Task, error) {
	var create Task
	err := p.pool.QueryRow(ctx, `INSERT INTO tasks (title, description,status) VALUES ($1, $2, $3) 
	RETURNING id, title, description, status, created_at, updated_at`, task.Title, task.Description, task.Status).Scan(
		&create.ID, &create.Title, &create.Description, &create.Status, &create.CreatedAt, &create.UpdatedAt)
	if err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}
	return create, nil
}

func (p *PostgresRepository) Update(ctx context.Context, task Task) (Task, error) {
	var updated Task

	err := p.pool.QueryRow(ctx, `UPDATE tasks SET title =$1,description=$2,status=$3,updated_at=NOW() WHERE id =$4 
	RETURNING id,title,description,status,created_at,updated_at`,
		task.Title, task.Description, task.Status, task.ID).Scan(
		&updated.ID, &updated.Title, &updated.Description, &updated.Status, &updated.CreatedAt, &updated.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrNotFound
		}
		return Task{}, fmt.Errorf("update task: %w", err)
	}
	return updated, nil
}

func (p *PostgresRepository) Delete(ctx context.Context, id int64) error {
	result, err := p.pool.Exec(ctx, `DELETE FROM tasks WHERE id =$1`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

var _ Repository = (*PostgresRepository)(nil)
