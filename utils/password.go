package utils

import "golang.org/x/crypto/bcrypt"

// HashPassword returns the bcrypt hash of the password using a cost that balances security and performance.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword compares the bcrypt hashed password with its possible plaintext equivalent.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
