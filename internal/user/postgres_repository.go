package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (p *PostgresRepository) Create(ctx context.Context, input User) (User, error) {
	var user User
	var pgErr *pgconn.PgError

	err := p.pool.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, $2) 
	RETURNING id, email, password_hash, created_at`, input.Email, input.PasswordHash).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "users_email_lower_idx" {
			return User{}, ErrEmailAlreadyExists
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (p *PostgresRepository) GetByEmail(ctx context.Context, input string) (User, error) {
	var user User

	err := p.pool.QueryRow(ctx, `SELECT id, email, password_hash, created_at FROM users WHERE lower(email) = lower($1)`,
		input).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("get user by email: %w", err)
	}

	return user, nil
}

var _ Repository = (*PostgresRepository)(nil)
