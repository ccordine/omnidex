package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func TestCodingPlanningAndParserMinificationAcceptsNecessaryLargeResponses(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.PlanningMutation, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
			config, err := modelconfig.Freeze(modelconfig.Config{"context_minification_model": "fixture-minifier"})
			if err != nil {
				t.Fatal(err)
			}
			repository := newWorkerTestRepository(pool, config)
			ctx := context.Background()
			if _, err := repository.EnqueueCodingJob(ctx, "Preserve the necessary facts.", t.TempDir()); err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "large-context-worker")
			if err != nil || claim == nil {
				t.Fatalf("claim: %v", err)
			}
			necessary := strings.TrimSpace(strings.Repeat("Preserve the failure/result and read/write facts for ARTIFACT_1. ", 100))
			provider := &exactEvidenceStationClient{}
			for range 32 {
				provider.fixtures = append(provider.fixtures, exactEvidenceStationFixture{candidate: necessary})
			}
			service := &Service{repo: repository, stationClient: provider, inferenceContextTokens: "8192", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
			runtime := &nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}
			background := strings.Repeat("Required fact for /opt/project/settings.toml. ", 300)
			call := codingagent.Assignment{Role: role, Response: "Exact new response.", PlanningResponse: "Exact new planning response."}
			if role == codingagent.Plan {
				call.PlanningConversation = []codingagent.PlanningMessage{{Role: "user", Text: background}}
			}
			if role == codingagent.PlanningMutation {
				call.ConstructionFacts = []codingagent.ConstructionFact{{Kind: "retained_context", Quote: background}}
			}
			if role == codingagent.Review {
				// Review background now belongs to the native review packet;
				// the response classifier intentionally receives no shared context.
				call.Findings = []codingagent.Finding{{Observation: background}}
			}
			result, err := compileCodingContextFixture(ctx, runtime, call)
			want := strings.ReplaceAll(necessary, "ARTIFACT_1", "/opt/project/settings.toml")
			if err != nil || result != want || provider.calls == 0 {
				t.Fatalf("large necessary context rejected/altered: bytes=%d calls=%d %v", len(result), provider.calls, err)
			}
			before := provider.calls
			if replay, err := compileCodingContextFixture(ctx, runtime, call); err != nil || replay != result || provider.calls != before {
				t.Fatalf("accepted context regenerated: calls=%d/%d %v", provider.calls, before, err)
			}
		})
	}
}
