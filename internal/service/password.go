package service

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the work factor for stored password hashes. 12 costs roughly
// a quarter of a second per hash on current hardware, which is acceptable for
// an operator login and slows offline cracking. Existing hashes keep the cost
// they were created with (bcrypt stores it in the hash).
const BcryptCost = 12

// MinPasswordLength applies to accounts created through cmd/admin.
const MinPasswordLength = 12

// ErrInvalidCredentials is the only error a failed login reports to clients,
// whatever the reason (unknown email, wrong password, no password, disabled).
var ErrInvalidCredentials = errors.New("invalid email or password")

// HashPassword returns a bcrypt hash of password at BcryptCost.
func HashPassword(password string) (string, error) {
	if len(password) > 72 {
		return "", fmt.Errorf("password must be at most 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// checkPassword compares password against hash in roughly constant time
// whether or not the account exists: with no hash it still runs a bcrypt
// comparison against a throwaway hash, so response timing does not reveal
// which emails are registered.
func checkPassword(hash *string, password string) bool {
	if hash == nil || *hash == "" {
		dummyHashOnce.Do(func() {
			dummyHash, _ = bcrypt.GenerateFromPassword([]byte("timing-equaliser"), BcryptCost)
		})
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(*hash), []byte(password)) == nil
}
