package worker

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/modelconfig"
)

// The observed failure survived a JSON-redactor unit test: the shared station
// redacted the escaped/chunked serialization a second time. Inspect the actual
// persisted model input, not merely the earlier helper's output.
func TestCapturedCodingContextsReachMinifierWithoutInventedArtifactBindings(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	for _, name := range []string{"expense", "wordcounter", "minification-boundary"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../../testdata/planning-gauntlet/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				ParserContext json.RawMessage                       `json:"mutation_parser_context"`
				Input         assemblyline.ContextMinificationInput `json:"minification_input"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			input := fixture.ParserContext
			if name == "minification-boundary" {
				parts := []string{}
				for _, authority := range fixture.Input.SelectedAuthorities {
					_, part, found := strings.Cut(authority.Content, "\n")
					if !found {
						t.Fatal("captured segment header missing")
					}
					parts = append(parts, part)
				}
				input = []byte(strings.Join(parts, ""))
			}
			var call codingagent.Assignment
			if err := json.Unmarshal(input, &call); err != nil {
				t.Fatal(err)
			}
			call.Role = codingagent.PlanningMutation
			contextJSON, _ := json.Marshal(codingagent.PromptContext(call))
			_, bindings, err := assemblyline.RedactContextArtifactJSON(contextJSON)
			if err != nil {
				t.Fatal(err)
			}
			allowed := map[string]bool{}
			for _, binding := range bindings {
				allowed[binding.Token] = true
			}
			pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
			configuration, err := modelconfig.Freeze(modelconfig.Config{"context_minification_model": "fixture-minifier"})
			if err != nil {
				t.Fatal(err)
			}
			repository := newWorkerTestRepository(pool, configuration)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "Preserve the exact planning obligations.", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "context-projection-worker")
			if err != nil || claim == nil {
				t.Fatalf("claim: %v", err)
			}
			provider := &exactEvidenceStationClient{}
			for range 64 {
				provider.fixtures = append(provider.fixtures, exactEvidenceStationFixture{candidate: "Retain the exact planning obligations and owner approval boundary."})
			}
			service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "8192", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
			runtime := &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}
			source := call.PlanningResponse
			compact, err := compileCodingContextFixture(ctx, runtime, call)
			if err != nil {
				t.Fatal(err)
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
			if name == "minification-boundary" {
				// This captured failure was driven by the new response itself,
				// not shared ledger history. It now remains exact parser input
				// and must never enter semantic minification at all.
				if err != nil || len(input) < 2048 || source == "" || call.PlanningResponse != source || strings.Contains(compact, source) || len(calls) != 0 || provider.calls != 0 {
					t.Fatalf("current parser source was lost/reinterpreted: source=%d calls=%d/%d %v", len(source), len(calls), provider.calls, err)
				}
				return
			}
			if err != nil || len(calls) == 0 {
				t.Fatalf("missing minifier evidence: %v", err)
			}
			artifacts := regexp.MustCompile(`ARTIFACT_[1-9][0-9]*`)
			for _, evidence := range calls {
				if strings.Contains(evidence.ModelInput, `\"`) || strings.Contains(evidence.ModelInput, `\n`) || strings.Contains(evidence.ModelInput, "Segment 1/") {
					t.Fatalf("serialized JSON/framing reached path redaction again: %s", evidence.ModelInput)
				}
				for _, token := range artifacts.FindAllString(evidence.ModelInput, -1) {
					if !allowed[token] {
						t.Fatalf("shared minifier invented unrestorable binding %s", token)
					}
				}
			}
		})
	}
}
