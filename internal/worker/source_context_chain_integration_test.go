package worker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestFreshSchemaSecondSourceCorrectionResumesImmediateParentContext(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for correction context lineage")
	}
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise two separately grounded source defects", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "context-chain-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "const total = left - right;\nreturn other;", nativeContext: "[11,22,33]"},
		{candidate: "left + right", nativeContext: "[11,22,33,44,55]"},
		{candidate: "total"},
	}}
	service := &Service{repo: repository, stationClient: client, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
	runtime := portableWorkerRuntime(&nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}, "context-chain")
	runtime.MaxAttempts = assemblyline.MaxSourceBodyAttempts
	input := assemblyline.FragmentGenerationInput{Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)", Behavior: "Return the sum of left and right."}
	source, err := runDirectCodingLanguageFragmentWorker(runtime, "fixture-model", directCodingLanguageGenerationJob{
		Subject: "sum-body", Input: input,
		Validate: func(input assemblyline.FragmentGenerationInput, body string) (string, error) {
			if strings.Contains(body, "left - right") {
				return "", sourceRecoverySumDefect(t, body)
			}
			if start := strings.Index(body, "other"); start >= 0 {
				defect, err := assemblyline.NewSourceBodyDefect(body, start, start+len("other"),
					"Which local value contains the required result?", fmt.Errorf("undeclared return value"))
				if err != nil {
					return "", err
				}
				return "", defect
			}
			return validateDirectCodingJavaScriptFragment(input, body)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "const total = left + right;") || !strings.Contains(source, "return total;") {
		t.Fatalf("source=%q", source)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || client.calls != 3 {
		t.Fatalf("call count=%d provider=%d", len(calls), client.calls)
	}
	assertExactEvidenceRequestContext(t, calls[0].ProviderRequest, "")
	assertExactEvidenceRequestContext(t, calls[1].ProviderRequest, "[11,22,33]")
	assertExactEvidenceRequestContext(t, calls[2].ProviderRequest, "[11,22,33,44,55]")
	for index := 1; index < len(calls); index++ {
		if calls[index].ParentCallEvidenceID != calls[index-1].ID || calls[index].Iteration != index+1 ||
			calls[index-1].Outcome == nil || calls[index-1].Outcome.Status != queue.LLMCallRejected {
			t.Fatalf("correction %d did not retain its exact parent", index)
		}
	}
	if calls[2].Outcome == nil || calls[2].Outcome.Status != queue.LLMCallAccepted ||
		calls[2].ModelInput != "Which local value contains the required result?\n\nother" ||
		calls[2].SourceBaseCandidate != "const total = left + right;\nreturn other;" {
		t.Fatalf("second correction did not preserve the accepted first splice: %#v", calls[2])
	}
}
