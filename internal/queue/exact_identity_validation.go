package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func validSHA256ID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix) &&
		len(value) == len(prefix)+sha256.Size*2 &&
		validSHA256Digest(strings.TrimPrefix(value, prefix))
}

func validSHA256Digest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
