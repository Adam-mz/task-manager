package user

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidEmail    = errors.New("invalid email")
	ErrInvalidPassword = errors.New(
		"password must contain between 15 and 128 characters",
	)
	ErrInvalidCredentials = errors.New("invalid email or password")
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
	}
}

func (s *Service) Register(ctx context.Context, email, password string) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(cleanEmail)
	if err != nil || parsed.Address != cleanEmail {
		return User{}, ErrInvalidEmail
	}
	length := utf8.RuneCountInString(password)
	if length < 15 || length > 128 {
		return User{}, ErrInvalidPassword
	}

	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	if err := ctx.Err(); err != nil {
		return User{}, err
	}

	return s.repository.Create(ctx, User{Email: cleanEmail, PasswordHash: hash})
}

func (s *Service) Authenticate(ctx context.Context, email, password string) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	found, err := s.repository.GetByEmail(ctx, cleanEmail)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			_, hashErr := hashPassword(password)
			if hashErr != nil {
				return User{}, hashErr
			}
			if err := ctx.Err(); err != nil {
				return User{}, err
			}
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	match, err := checkPassword(password, found.PasswordHash)
	if err != nil {
		return User{}, err
	}
	if err := ctx.Err(); err != nil {
		return User{}, err
	}
	if !match {
		return User{}, ErrInvalidCredentials
	}
	return found, nil
}
