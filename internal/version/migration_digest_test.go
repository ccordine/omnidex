package version

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestDefaultMigrationDigestMatchesCheckedManifest(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	want := hex.EncodeToString(digest[:])
	if got := strings.TrimSpace(MigrationsSHA256); got != want {
		t.Fatalf("default migration digest=%q want %q", got, want)
	}
}
