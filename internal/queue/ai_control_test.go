package queue

import (
	"os"
	"strings"
	"testing"
)

func TestGlobalAIPauseIsTransactionalAndStrict(t *testing.T) {
	raw, err := os.ReadFile("ai_control.go")
	if err != nil {
		t.Fatalf("read AI control source: %v", err)
	}
	source := string(raw)
	for _, required := range []string{
		"func (r *Repository) PauseAI(",
		"BeginTx(ctx",
		"cancelJobTx(ctx, tx",
		"tx.Commit(ctx)",
		"decoder.DisallowUnknownFields()",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("AI control authority missing %q", required)
		}
	}
	for _, forbidden := range []string{"_ = json.Unmarshal", "ListJobIDsByStatuses"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("AI control retains non-atomic or silent path %q", forbidden)
		}
	}
}
