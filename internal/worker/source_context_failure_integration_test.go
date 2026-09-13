package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestFreshSchemaSourceCorrectionRequiresUsableParentContextBeforeDispatch(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for retained-context failure coverage")
	}
	for name, nativeContext := range map[string]string{
		"missing": "", "null": "null", "invalid token": "[1,null]",
	} {
		t.Run(name, func(t *testing.T) {
			_, repository := freshWorkerEvidenceRepository(t, databaseURL)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "exercise missing continuation capability", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "context-failure-worker")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim=%#v err=%v", claim, err)
			}
			client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
				{candidate: "const total = left - right;\nreturn total;", nativeContext: nativeContext},
				{candidate: "A"},
			}}
			work := directCodingLanguageGenerationJob{
				Subject: "sum-body", Input: assemblyline.FragmentGenerationInput{
					Language: "javascript", Dialect: "ECMAScript 2022",
					Signature: "function Sum(left, right)", Behavior: "Return the sum of left and right.",
				},
				Validate: func(_ assemblyline.FragmentGenerationInput, body string) (string, error) {
					return "", sourceRecoverySumDefect(t, body)
				},
			}
			// Reconstruct the runtime to prove the same persisted response fails
			// without acquiring a new initial response or a correction response.
			for attempt := 0; attempt < 2; attempt++ {
				service := &Service{repo: repository, stationClient: client, inferenceContextTokens: "8192",
					runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
				runtime := portableWorkerRuntime(&nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}, "context-failure")
				runtime.MaxAttempts = assemblyline.MaxSourceBodyAttempts
				_, err := runDirectCodingLanguageFragmentWorker(runtime, "fixture-model", work)
				if err == nil || !strings.Contains(err.Error(), "retained model context") || client.calls != 1 {
					t.Fatalf("reconstruction %d calls=%d err=%v", attempt, client.calls, err)
				}
			}
			// Missing continuation capability does not invalidate a separate
			// leaf whose accepted response does not require correction.
			service := &Service{repo: repository, stationClient: client, inferenceContextTokens: "8192",
				runtimeEventChannels: make(map[int64]runtimeEventChannelBinding)}
			runtime := portableWorkerRuntime(&nativeRuntimeV3{svc: service, ctx: ctx, claim: claim}, "independent-value")
			classification, err := assemblyline.NewApplicationClassificationJob(
				assemblyline.ApplicationClassificationInput{UserRequest: "classify an independent interface"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := runtime.Execute(classification, "fixture-model")
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Finalize(classification, result, nil); err != nil {
				t.Fatal(err)
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(calls) != 2 || client.calls != 2 || calls[0].Outcome == nil || calls[1].Outcome == nil ||
				calls[0].Outcome.Status != queue.LLMCallRejected || calls[1].Outcome.Status != queue.LLMCallAccepted ||
				calls[0].Iteration != 1 || calls[1].Iteration != 1 {
				t.Fatalf("missing context changed accepted state or created a continuation: %#v", calls)
			}
		})
	}
}
