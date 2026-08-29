package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestSHA256IdentityValidationIsExactAndTaskNeutral(t *testing.T) {
	digest := queueTestSHA256("exact identity")
	if !validSHA256Digest(digest) ||
		!validSHA256ID("workspace_mutation_"+digest, "workspace_mutation_") {
		t.Fatal("exact lowercase SHA-256 identity was rejected")
	}
	if validSHA256Digest(strings.ToUpper(digest)) ||
		validSHA256ID("repository_mutation_"+digest, "workspace_mutation_") {
		t.Fatal("non-exact SHA-256 identity was accepted")
	}
}

func queueTestSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
