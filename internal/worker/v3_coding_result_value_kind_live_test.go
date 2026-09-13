package worker

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

func TestLiveResultValueKindsStayAtTheLocalValueBoundary(t *testing.T) {
	model := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_GO_TEST_MODEL"))
	if model == "" {
		t.Skip("OMNIDEX_TEST_GO_TEST_MODEL is not set")
	}
	endpoint := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_URL"))
	limit, err := strconv.Atoi(os.Getenv("OMNIDEX_TEST_OLLAMA_CONTEXT"))
	if endpoint == "" || err != nil || limit <= 0 {
		t.Fatal("live endpoint and positive context limit required")
	}
	client := ollama.New(endpoint, model, "", llm.MaximumModelRequestDuration)
	for _, fixture := range []struct {
		behavior string
		kind     assemblyline.ApplicationResultValueKind
	}{
		{"Print the product of the two integer arguments.", assemblyline.ApplicationResultInteger},
		{"Print the supplied text unchanged.", assemblyline.ApplicationResultText},
	} {
		input := assemblyline.ApplicationResultValueKindInput{Requirement: fixture.behavior}
		job, err := assemblyline.NewApplicationResultValueKindJob(input)
		if err != nil {
			t.Fatal(err)
		}
		kind, err := runDirectCodingSemanticLeafCall(liveSourceBodyRuntime(t, client, model, limit), model, "application_result_value_kind", job, nil,
			func(raw string) (assemblyline.ApplicationResultValueKind, error) {
				return assemblyline.DecodeApplicationResultValueKind(input, raw)
			})
		if err != nil || kind != fixture.kind {
			t.Fatalf("kind=%s expected=%s error=%v", kind, fixture.kind, err)
		}
	}
}
