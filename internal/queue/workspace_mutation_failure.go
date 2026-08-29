package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxWorkspaceMutationErrorBytes = 64 * 1024

func exactWorkspaceMutationError(source error) (string, error) {
	if source == nil {
		return "", fmt.Errorf("workspace mutation state transition requires one exact failure")
	}
	message := source.Error()
	if message == "" || message != strings.TrimSpace(message) || !utf8.ValidString(message) ||
		strings.ContainsRune(message, '\x00') || len(message) > maxWorkspaceMutationErrorBytes {
		return "", fmt.Errorf("workspace mutation failure is not exact bounded PostgreSQL text: %w", source)
	}
	return message, nil
}

func workspaceMutationFailureSHA(message string) string {
	digest := sha256.Sum256([]byte(message))
	return hex.EncodeToString(digest[:])
}
