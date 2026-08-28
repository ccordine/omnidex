package architecture

import (
	"os"
	"regexp"
	"testing"
)

func TestPostgresImageIsBoundToOneImmutableDigest(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?m)^    image: pgvector/pgvector@sha256:[0-9a-f]{64}$`)
	if matches := pattern.FindAll(raw, -1); len(matches) != 1 {
		t.Fatalf("postgres image digest authorities=%d, want exactly 1", len(matches))
	}
}
