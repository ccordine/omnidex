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

func TestFreshSchemaSourceBodyCorrectionContinuesSamePersistedContextWithoutBlockingAnotherJob(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for source-body continuation evidence coverage")
	}
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise persisted source-body continuation", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "source-body-continuation-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "const total = left - right;\nreturn total;", nativeContext: "[11,22,33]"},
		{candidate: "A", nativeContext: "[99,88]"},
		{candidate: "left + right"},
	}}
	service := &Service{
		repo: repository, stationClient: client, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	base := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service, ctx: ctx, claim: claim,
	}, "source-body-integration")
	runtime := base
	runtime.MaxAttempts = assemblyline.MaxSourceBodyAttempts
	correct := base.Correct
	runtime.Correct = func(
		job assemblyline.PortableJob,
		modelName string,
		correction assemblyline.SourceBodyCorrection,
	) (assemblyline.PortableResult, error) {
		classification, err := assemblyline.NewApplicationClassificationJob(
			assemblyline.ApplicationClassificationInput{
				UserRequest: "classify an unrelated interface",
			},
		)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if _, err := runDirectCodingSemanticLeafCall(
			base, modelName, "unrelated-classification", classification, nil,
			func(candidate string) (string, error) {
				decoded, err := assemblyline.DecodeApplicationClassification(
					assemblyline.ApplicationClassificationInput{
						UserRequest: "classify an unrelated interface",
					},
					candidate,
				)
				if err != nil {
					return "", err
				}
				return string(decoded.Surface), nil
			},
		); err != nil {
			return assemblyline.PortableResult{}, err
		}
		// Configuration may change while this persisted job is alive. The
		// correction must retain the parent's frozen native model context.
		service.inferenceContextTokens = "16384"
		return correct(job, modelName, correction)
	}
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)",
		Behavior:  "Return the sum of left and right.",
	}
	source, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "sum-body", Input: input,
			Validate: func(
				input assemblyline.FragmentGenerationInput,
				body string,
			) (string, error) {
				const wrong = "left - right"
				if start := strings.Index(body, wrong); start >= 0 {
					defect, defectErr := assemblyline.NewSourceBodyDefect(
						body,
						start,
						start+len(wrong),
						"Which expression computes the required sum?",
						fmt.Errorf("subtraction does not satisfy the required sum"),
					)
					if defectErr != nil {
						return "", defectErr
					}
					return "", defect
				}
				return validateDirectCodingJavaScriptFragment(input, body)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(source, input.Signature) != 1 ||
		!strings.Contains(source, "const total = left + right;") ||
		!strings.Contains(source, "return total;") {
		t.Fatalf("code-assembled source=%q", source)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[0].Outcome == nil || calls[1].Outcome == nil ||
		calls[2].Outcome == nil ||
		calls[0].WorkKind != string(assemblyline.WorkFragmentGeneration) ||
		calls[0].Iteration != 1 || calls[0].Outcome.Status != queue.LLMCallRejected ||
		calls[1].WorkKind != string(assemblyline.WorkApplicationClassify) ||
		calls[1].Iteration != 1 || calls[1].Outcome.Status != queue.LLMCallAccepted ||
		calls[2].WorkInput != nil || len(calls[0].WorkInput) == 0 || calls[2].Model != calls[0].Model ||
		calls[2].Iteration != 2 || calls[2].OutputContinuation != 0 ||
		calls[2].ParentCallEvidenceID != calls[0].ID ||
		calls[2].OutputLimitReached || calls[2].Outcome.Status != queue.LLMCallAccepted ||
		calls[0].ContextTokens != 8192 || calls[2].ContextTokens != calls[0].ContextTokens ||
		calls[2].SourceBaseCandidate != "const total = left - right;\nreturn total;" ||
		calls[2].SourceStartByte != len("const total = ") ||
		calls[2].SourceEndByte != len("const total = left - right") {
		t.Fatalf("source-body continuation evidence=%#v", calls)
	}
	const wantedCorrectionInput = "Which expression computes the required sum?\n\nleft - right"
	assertExactEvidenceRequestContext(t, calls[0].ProviderRequest, "")
	assertExactEvidenceRequestContext(t, calls[1].ProviderRequest, "")
	assertExactEvidenceRequestContext(t, calls[2].ProviderRequest, "[11,22,33]")
	if len(client.prepared) != 3 || client.prepared[2].Prompt != wantedCorrectionInput ||
		client.prepared[0].MaxOutputTokens != -1 ||
		client.prepared[2].MaxOutputTokens != -1 ||
		strings.Contains(client.prepared[2].Prompt, "const total") ||
		strings.Contains(client.prepared[2].Prompt, "return total") ||
		strings.Contains(client.prepared[2].Prompt, input.Signature) {
		t.Fatalf("correction model input was not the exact mutable span: %#v", client.prepared)
	}
}
