package worker

import (
	"os"
	"strings"
	"testing"
)

func TestProjectDebuggerWorkerHasNoPartialSuccessPath(t *testing.T) {
	for _, path := range []string{"project_debugger.go", "scrum_card_llm_enqueue.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"_ = s.saveDebuggerLastRun",
			"_ = json.Unmarshal",
			`"qwen3:4b-thinking"`,
			`"llama3.2"`,
			"s.repo.EnqueueJob(ctx, fmt.Sprintf(\"Generate planning ticket",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s contains partial or fallback path %q", path, forbidden)
			}
		}
	}
}
