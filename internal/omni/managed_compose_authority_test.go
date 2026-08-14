package omni

import (
	"os"
	"strings"
	"testing"
)

func TestManagedComposeRequiresExplicitCoreURLAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "CORE_URL: ${CORE_URL:?CORE_URL must be configured}") {
		t.Fatal("managed Compose must require the exact CORE_URL from the validated managed environment")
	}
	for _, forbidden := range []string{
		"CORE_URL: ${CORE_URL:-", "CORE_URL: http://localhost", "CORE_URL: https://omni.worknet",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("managed Compose contains forbidden CORE_URL fallback %q", forbidden)
		}
	}
}
