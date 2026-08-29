package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyAggregateJobInspectionIsAbsent(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	checks := map[string][]string{
		"internal/api/server_llm.go":                   {`/inspection`, "inspectJob"},
		"internal/client/client.go":                    {"func (c *Client) Inspect"},
		"internal/model/model.go":                      {"type JobInspection struct"},
		"internal/queue/repository_mind_inspection.go": {"GetJobInspection"},
		"internal/queue/repository.go":                 {"ListHistoricalArtifactsByJob", "ListHistoricalEvidenceByJob"},
		"internal/queue/llm_evidence.go":               {"ListLLMCallEvidenceForJob", "OFFSET $3"},
		"cmd/cli/job_query_commands.go":                {`fs.Bool("inspect"`, ".Inspect("},
	}
	for name, forbidden := range checks {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range forbidden {
			if strings.Contains(string(raw), value) {
				t.Fatalf("legacy job inspection token %q remains in %s", value, name)
			}
		}
	}
}
