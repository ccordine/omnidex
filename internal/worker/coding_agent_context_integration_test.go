package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
)

// Fixed responses verify the production compiler/station/PG boundary, not live
// semantic quality or a workload-building qualification.
func TestCodingContextCompilerMinificationForLargeNativeAndParserInputs(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.PlanningMutation} {
		t.Run(string(role), func(t *testing.T) {
			pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
			config, err := modelconfig.Freeze(modelconfig.Config{"context_minification_model": "fixture-minification"})
			if err != nil {
				t.Fatal(err)
			}
			repository := newWorkerTestRepository(pool, config)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "Retain the exact task facts.", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "coding-context-test-worker")
			if err != nil || claim == nil {
				t.Fatalf("claim: %v", err)
			}
			fixtures := make([]exactEvidenceStationFixture, 128)
			for index := range fixtures {
				fixtures[index].candidate = "Keep the required fact for ARTIFACT_1 and the owner's answer."
			}
			provider := &exactEvidenceStationClient{fixtures: fixtures}
			service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "8192", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
			runtime := &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}
			call := codingagent.Assignment{Role: role}
			if role == codingagent.Execute {
				call.Task = &codingagent.Task{ID: "selected", Goal: "Keep /opt/project/config.toml exactly.", Acceptance: []model.TaskAcceptance{{ID: "owner-answer", Text: "Keep the owner's answer.", Verification: []string{"Inspect retained answer."}}}}
			} else {
				call.ConstructionFacts = []codingagent.ConstructionFact{{Kind: "retained_context", Quote: "Keep /srv/service/settings.yaml exactly. The owner's answer is still needed."}}
				call.PlanningResponse = "The exact new response remains current parser input."
			}
			small, err := compileCodingContextFixture(ctx, runtime, call)
			if err != nil || provider.calls != 0 || !strings.Contains(small, "owner") {
				t.Fatalf("small context needlessly inferred or lost facts: %q calls=%d %v", small, provider.calls, err)
			}
			if role == codingagent.Execute {
				call.Task.Goal += strings.Repeat(" Repeated task-local history.", 7000)
			} else {
				call.ConstructionFacts[0].Quote += strings.Repeat(" Repeated retained detail.", 7000)
			}
			compact, err := compileCodingContextFixture(ctx, runtime, call)
			if err != nil || len(compact) > 2048 || provider.calls == 0 {
				t.Fatalf("large context did not minify: %d calls=%d %v", len(compact), provider.calls, err)
			}
			path := "/opt/project/config.toml"
			if role == codingagent.PlanningMutation {
				path = "/srv/service/settings.yaml"
			}
			if !strings.Contains(compact, path) || strings.Contains(compact, "ARTIFACT_") || !strings.Contains(compact, "owner's answer") {
				t.Fatalf("compact context lost exact bindings: %q", compact)
			}
			before := provider.calls
			if replay, err := compileCodingContextFixture(ctx, runtime, call); err != nil || replay != compact || provider.calls != before {
				t.Fatalf("minification repeated accepted inference: calls=%d/%d %v", before, provider.calls, err)
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
			if err != nil || len(calls) != before {
				t.Fatalf("minification lost PG evidence: calls=%d want=%d %v", len(calls), before, err)
			}
			for _, evidence := range calls {
				if strings.Contains(evidence.ModelInput, path) {
					t.Fatal("path identity leaked into the semantic minifier")
				}
				if call.PlanningResponse != "" && strings.Contains(evidence.ModelInput, call.PlanningResponse) {
					t.Fatal("current parser source was reinterpreted by shared-context minification")
				}
			}
		})
	}
}

// Compose existing projection/redaction/compiler boundaries for these captured
// component fixtures. Native execution no longer has a minification adapter;
// this helper is not proof that production native packets are summarized.
func compileCodingContextFixture(ctx context.Context, runtime *nativeRuntimeV3, call codingagent.Assignment) (string, error) {
	raw, err := json.Marshal(codingagent.PromptContext(call))
	if err != nil {
		return "", err
	}
	redacted, bindings, err := assemblyline.RedactContextArtifactJSON(raw)
	if err != nil {
		return "", err
	}
	var value any
	if err := json.Unmarshal([]byte(redacted), &value); err != nil {
		return "", err
	}
	var parts []string
	var collect func(any) error
	collect = func(value any) error {
		switch v := value.(type) {
		case string:
			if strings.HasPrefix(strings.TrimSpace(v), "{") && json.Valid([]byte(v)) {
				var nested any
				if err := json.Unmarshal([]byte(v), &nested); err != nil {
					return err
				}
				return collect(nested)
			}
			if v != "" {
				parts = append(parts, v)
			}
		case []any:
			for _, item := range v {
				if err := collect(item); err != nil {
					return err
				}
			}
		case map[string]any:
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if err := collect(v[key]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := collect(value); err != nil {
		return "", err
	}
	if len(parts) == 0 {
		return "", nil
	}
	set, err := buildContextCandidateSet([]queue.ContextSearchRecord{{Namespace: "coding_context_fixture", SourceID: "captured", Content: strings.Join(parts, "\n")}}, nil)
	if err != nil {
		return "", err
	}
	stations := portableObjectiveContextSieveStations{runtime: runtime}
	result, err := contextcompiler.Compile(ctx, contextcompiler.Request{ExactInstruction: "Retain the required task facts and owner's answer.", ModelInstruction: "Retain the required task facts and owner's answer.", KnownArtifactPaths: []string{}}, sessionTaskContextProvider{set: set}, contextcompiler.Stations{Minification: stations})
	if err != nil {
		return "", err
	}
	if len(result.Context.Capsules) != 1 {
		return "", fmt.Errorf("expected one compiled context capsule")
	}
	return assemblyline.RestoreArtifactIdentities(result.Context.Capsules[0].Content, bindings)
}
