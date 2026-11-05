package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const (
	argonMemory      uint32 = 64 * 1024
	argonIterations  uint32 = 3
	argonParallelism uint8  = 2
	argonSaltLength         = 16
	argonKeyLength          = 32
)

var (
	ErrEmptyPassword   = errors.New("password cannot be empty")
	ErrPasswordTooLong = errors.New("password exceeds 1024 bytes")
)

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}

	if len([]byte(password)) > 1024 {
		return "", ErrPasswordTooLong
	}

	salt := make([]byte, argonSaltLength)

	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argonIterations,
		argonMemory,
		argonParallelism,
		argonKeyLength,
	)

	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonIterations,
		argonParallelism,
		encodedSalt,
		encodedHash,
	), nil
}

func VerifyPassword(password, encoded string) bool {
	if password == "" || len([]byte(password)) > 1024 {
		return false
	}

	// Support existing bcrypt password hashes.
	if strings.HasPrefix(encoded, "$2a$") ||
		strings.HasPrefix(encoded, "$2b$") ||
		strings.HasPrefix(encoded, "$2y$") {

		return bcrypt.CompareHashAndPassword(
			[]byte(encoded),
			[]byte(password),
		) == nil
	}

	// Verify modern Argon2id hashes.
	return verifyArgon2id(password, encoded)
}

func NeedsPasswordUpgrade(encoded string) bool {
	return strings.HasPrefix(encoded, "$2")
}

func verifyArgon2id(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}

	if parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}

	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false
	}
	readParam := func(value, prefix string) (uint64, bool) {
		if !strings.HasPrefix(value, prefix) {
			return 0, false
		}
		n, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, 32)
		return n, err == nil
	}
	memoryValue, okM := readParam(params[0], "m=")
	iterationsValue, okT := readParam(params[1], "t=")
	parallelismValue, okP := readParam(params[2], "p=")
	if !okM || !okT || !okP || memoryValue < 8*1024 || memoryValue > 256*1024 ||
		iterationsValue < 1 || iterationsValue > 10 || parallelismValue < 1 || parallelismValue > 8 {
		return false
	}
	memory := uint32(memoryValue)
	iterations := uint32(iterationsValue)
	parallelism := uint8(parallelismValue)

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	if len(salt) < 8 || len(salt) > 64 || len(hash) < 16 || len(hash) > 64 {
		return false
	}

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memory,
		parallelism,
		uint32(len(hash)),
	)

	return subtle.ConstantTimeCompare(hash, computedHash) == 1
}
