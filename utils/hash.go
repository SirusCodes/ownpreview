package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func SmallAlphaNumericHash(input string, length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("length must be positive")
	}

	// Create SHA-256 hash
	hash := sha256.Sum256([]byte(input))

	encoded := hex.EncodeToString(hash[:])

	// Ensure length does not exceed available characters
	if length > len(encoded) {
		length = len(encoded)
	}

	return encoded[:length], nil
}
