package auth

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid access token")

const (
	tokenIssuer   = "task-manager"
	tokenAudience = "task-manager-api"
)

type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenManager(secret []byte, ttl time.Duration) (*TokenManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("secret must be at least 32 bytes")
	}
	if ttl <= 0 {
		return nil, errors.New("ttl must be positive")
	}
	return &TokenManager{
		secret: bytes.Clone(secret),
		ttl:    ttl,
	}, nil
}

func (m *TokenManager) Generate(userID int64) (string, error) {
	if userID <= 0 {
		return "", errors.New("user ID must be positive")
	}
	now := time.Now().UTC()

	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		Issuer:    tokenIssuer,
		Audience:  jwt.ClaimStrings{tokenAudience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}

	return signed, nil
}

func (m *TokenManager) Parse(tokenString string) (int64, error) {
	var claims jwt.RegisteredClaims

	parsed, err := jwt.ParseWithClaims(
		tokenString,
		&claims,
		func(_ *jwt.Token) (any, error) {
			return m.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithAudience(tokenAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)

	if err != nil || parsed == nil || !parsed.Valid {
		return 0, ErrInvalidToken
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)

	if err != nil || userID <= 0 {
		return 0, ErrInvalidToken
	}

	return userID, nil
}
