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

func TestLiveInputChannelsStayAtTheLocalInputBoundary(t *testing.T) {
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
		kind     assemblyline.ApplicationInputSource
	}{
		{"Print the product of the two integer arguments.", assemblyline.ApplicationInputArguments},
		{"Print the text read from standard input unchanged.", assemblyline.ApplicationInputStandardInput},
	} {
		input := assemblyline.ApplicationInputSourceInput{Requirement: fixture.behavior}
		job, err := assemblyline.NewApplicationInputSourceJob(input)
		if err != nil {
			t.Fatal(err)
		}
		kind, err := runDirectCodingSemanticLeafCall(liveSourceBodyRuntime(t, client, model, limit), model, "application_input_source", job, nil,
			func(raw string) (assemblyline.ApplicationInputSource, error) {
				return assemblyline.DecodeApplicationInputSource(input, raw)
			})
		if err != nil || kind != fixture.kind {
			t.Fatalf("kind=%s expected=%s error=%v", kind, fixture.kind, err)
		}
	}
}
