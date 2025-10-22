package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hash generates a SHA-256 hash of the input string
func Hash(input string) string {
	hasher := sha256.New()
	hasher.Write([]byte(input))
	return hex.EncodeToString(hasher.Sum(nil))
}
