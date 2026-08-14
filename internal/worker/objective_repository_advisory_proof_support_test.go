package worker

import (
	"context"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
)

type advisoryProofCase struct {
	name, objectiveID, requirement string
	evidence                       []assemblyline.GroundedEvidenceCapsule
	incorrect, correct             string
	advice, relevanceNeedle        string
	provider, model                string
}

type advisoryProofRun struct {
	result   objectiveRepositoryGroundedResult
	station  *advisoryProofStation
	mutation *advisoryProofMutationSpy
}

type advisoryProofStation struct {
	answerText, incorrect, corrected, relevanceNeedle string
	mutation                                          *advisoryProofMutationSpy
	events                                            []string
	reviewInputs                                      []assemblyline.RepositoryGroundedReviewInput
	correctionInputs                                  []assemblyline.RepositoryGroundedCorrectionInput
	mutationCallsAtDetection                          int
}

func (station *advisoryProofStation) Answer(
	_ context.Context, input assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "answer")
	ids := make([]string, len(input.Evidence))
	for index := range input.Evidence {
		ids[index] = input.Evidence[index].ID
	}
	return assemblyline.GroundedAnswerDecision{
		Schema: assemblyline.GroundedAnswerSchemaV1, RequirementID: input.RequirementID,
		Text: station.answerText, EvidenceIDs: ids,
	}, objectiveStationReceipt{Calls: 1}, nil
}

func (station *advisoryProofStation) Review(
	_ context.Context, input assemblyline.RepositoryGroundedReviewInput,
) (assemblyline.RepositoryGroundedReviewDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "review")
	station.reviewInputs = append(station.reviewInputs, cloneRepositoryReviewInput(input))
	decision := assemblyline.RepositoryGroundedReviewDecision{
		Schema:  assemblyline.RepositoryGroundedReviewSchemaV1,
		Outcome: assemblyline.RepositoryGroundedReviewNone,
	}
	if input.AnswerText == station.incorrect && len(input.AdvisoryCapsules) == 1 &&
		strings.Contains(strings.ToLower(input.AdvisoryCapsules[0].Content), station.relevanceNeedle) {
		station.mutationCallsAtDetection = station.mutation.calls
		decision.Outcome = assemblyline.RepositoryGroundedReviewIssue
		decision.IssueKind = assemblyline.RepositoryGroundedRequirementGap
		decision.Detail = "The candidate omitted the advisory-highlighted constraint present in cited evidence."
	}
	return decision, objectiveStationReceipt{Calls: 1}, nil
}

func (station *advisoryProofStation) Correct(
	_ context.Context, input assemblyline.RepositoryGroundedCorrectionInput,
) (assemblyline.RepositoryGroundedCorrectionDecision, objectiveStationReceipt, error) {
	station.events = append(station.events, "correct")
	station.correctionInputs = append(station.correctionInputs, cloneRepositoryCorrectionInput(input))
	return assemblyline.RepositoryGroundedCorrectionDecision{Text: station.corrected},
		objectiveStationReceipt{Calls: 1}, nil
}

type advisoryProofProvider struct {
	raw, provider, model    string
	calls                   int
	modelSelectedOperations int
}

func (provider *advisoryProofProvider) Generate(
	_ context.Context, _ objectiveadvisory.GenerateRequest,
) (objectiveadvisory.Generation, error) {
	provider.calls++
	return objectiveadvisory.Generation{
		FinalText: provider.raw, EffectiveProvider: provider.provider,
		EffectiveModel: provider.model, ModelDigest: strings.Repeat("a", 64),
		Quantization: "proof", PromptTokens: 100,
		OutputTokens: (len(provider.raw) + 3) / 4, Duration: time.Millisecond,
		FinishReason: "stop",
	}, nil
}

func (provider *advisoryProofProvider) SelectOperation() {
	provider.modelSelectedOperations++
}

type advisoryProofEmbedder struct{ calls int }

func (embedder *advisoryProofEmbedder) Embedding(
	_ context.Context, _ string,
) ([]float64, error) {
	embedder.calls++
	return []float64{1, 1}, nil
}

type advisoryProofMutationSpy struct{ calls int }

func (spy *advisoryProofMutationSpy) Mutate() { spy.calls++ }

func newAdvisoryProofRuntime(
	mode objectiveadvisory.Mode,
	proof advisoryProofCase,
	provider *advisoryProofProvider,
) *objectiveadvisory.Runtime {
	runtime, err := objectiveadvisory.New(objectiveadvisory.Config{
		Mode: mode, MinimumRelevance: 0.9, MaxSelectedCapsules: 1,
		Sources: []objectiveadvisory.SourceConfig{{
			ID: "proof-source", Provider: proof.provider, Model: proof.model,
			Sampling: objectiveadvisory.SamplingConfig{Temperature: 0.4},
			Budget: objectiveadvisory.Budget{
				MaxInputBytes:  objectiveadvisory.MaxProjectionBytes + 4*1024,
				MaxOutputBytes: objectiveadvisory.MaxRawTextBytes, MaxOutputTokens: 4096,
			},
		}},
	}, provider, &advisoryProofEmbedder{}, func() time.Time {
		return time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		panic(err)
	}
	return runtime
}
