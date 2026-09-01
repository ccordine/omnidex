package omni

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func workspaceHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:16]
}
