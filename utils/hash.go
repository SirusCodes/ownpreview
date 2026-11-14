package utils

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"regexp"
)

func SmallAlphaNumericHash(input string, length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("length must be positive")
	}

	// Create SHA-256 hash
	hash := sha256.Sum256([]byte(input))

	// Encode to Base64 (contains letters, numbers, +, /)
	encoded := base64.StdEncoding.EncodeToString(hash[:])

	// Remove non-alphanumeric characters (+, /, =)
	re := regexp.MustCompile(`[^a-zA-Z0-9]`)
	clean := re.ReplaceAllString(encoded, "")

	// Ensure length does not exceed available characters
	if length > len(clean) {
		length = len(clean)
	}

	return clean[:length], nil
}
