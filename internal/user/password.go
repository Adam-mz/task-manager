package user

import (
	"fmt"

	"github.com/alexedwards/argon2id"
)

var passwordHashParams = &argon2id.Params{
	Memory:      19 * 1024, // 19 MiB, значение задаётся в KiB
	Iterations:  2,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

func hashPassword(password string) (string, error) {
	hash, err := argon2id.CreateHash(password, passwordHashParams)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return hash, nil
}

func checkPassword(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, fmt.Errorf("check password: %w", err)
	}
	return match, nil
}
