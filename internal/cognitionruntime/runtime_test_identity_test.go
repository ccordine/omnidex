package cognitionruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func runtimeDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
