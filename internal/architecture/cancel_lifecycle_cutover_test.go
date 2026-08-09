package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCancellationHasNoPositionalLifecycleAPI(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	checks := map[string][]string{
		"internal/queue/repository_cancel.go": {
			"CancelJob(ctx context.Context, jobID int64, reason string)",
			"cancelJobTx(ctx context.Context, tx pgx.Tx, jobID int64, reason string)",
		},
		"internal/client/client.go": {
			"Cancel(ctx context.Context, id int64, reason string)",
		},
		"internal/api/server.go": {
			"type cancelRequest struct {\n\tReason string",
		},
	}
	for name, forbidden := range checks {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Fatalf("positional cancellation token %q remains in %s", token, name)
			}
		}
	}
}

func TestBrowserCancellationCarriesLifecycleIdentity(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(
		"..", "api", "web", "src", "lib", "chat_jobs_coordinator.ts",
	))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "jsonRequest({ operation_id: operationID, reason })") {
		t.Fatal("browser cancellation omits its explicit lifecycle operation identity")
	}
}
