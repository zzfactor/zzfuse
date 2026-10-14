package auth

import (
	"os"
	"strconv"

	"golang.org/x/crypto/bcrypt"
)

func getPasswordPepper() string {
	if v := os.Getenv("TRAZR_PASSWORD_PEPPER"); v != "" {
		return v
	}
	return os.Getenv("PASSWORD_PEPPER")
}

func getBcryptCost() int {
	if v := os.Getenv("BCRYPT_COST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= bcrypt.MinCost && n <= bcrypt.MaxCost {
			return n
		}
	}
	// Stronger default
	return 12
}

// HashPassword hashes a plaintext password using bcrypt and an optional server-side pepper.
func HashPassword(password string) (string, error) {
	peppered := password + getPasswordPepper()
	hash, err := bcrypt.GenerateFromPassword([]byte(peppered), getBcryptCost())
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword compares a bcrypt hashed password with its possible plaintext equivalent.
func CheckPassword(hash, password string) error {
	peppered := password + getPasswordPepper()
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(peppered))
}
