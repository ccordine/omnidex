package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestHistoricalStackConstraintReplayPreservesV1Input(t *testing.T) {
	legacyInput := struct {
		ProductContext       string                                          `json:"product_context"`
		AcceptedRequirements []string                                        `json:"accepted_requirements"`
		Candidates           []assemblyline.ApplicationProjectStackCandidate `json:"candidates"`
	}{
		ProductContext:       "historical command-line report",
		AcceptedRequirements: []string{"Use Go for the report"},
		Candidates: []assemblyline.ApplicationProjectStackCandidate{{
			CandidateID:     "STACK_CANDIDATE_1",
			TechnicalFormat: "Go for a command-line application",
		}},
	}
	payload, err := json.Marshal(legacyInput)
	if err != nil {
		t.Fatal(err)
	}
	job := assemblyline.PortableJob{
		Schema:  assemblyline.PortableJobSchemaV2,
		Kind:    assemblyline.WorkApplicationProjectStackConstraint,
		Payload: payload,
	}
	job.ID = historicalPortableJobID(job)

	for _, renderer := range []string{
		assemblyline.HistoricalPortableRendererV5,
		assemblyline.HistoricalPortableRendererV6,
	} {
		t.Run(renderer, func(t *testing.T) {
			gap := historicalStackReplayGap(t, job, renderer)
			point := queue.StationCallReplayPoint{
				Gap: gap, Call: replayTestCall(t, gap),
			}
			boundary, err := validateExactStationReplayPoint(point)
			if err != nil {
				t.Fatal(err)
			}
			if string(boundary.Job.Payload) != string(payload) ||
				boundary.Prompt != gap.Prompt {
				t.Fatalf("historical stack boundary=%+v", boundary)
			}
			artifact, err := replayExactStationArtifactForRenderer(
				boundary.Job, renderer, "STACK_CANDIDATE_1",
			)
			if err != nil || artifact.Kind != string(job.Kind) {
				t.Fatalf("historical stack artifact=%+v error=%v", artifact, err)
			}
		})
	}
}

func TestHistoricalStackConstraintReplayPreservesV7Input(t *testing.T) {
	job, err := assemblyline.NewApplicationProjectStackConstraintJob(
		assemblyline.ApplicationProjectStackConstraintInput{
			UserRequest: "Build a Go command-line report.",
			Candidates: []assemblyline.ApplicationProjectStackCandidate{{
				CandidateID:     "STACK_CANDIDATE_1",
				TechnicalFormat: "Go for a command-line application",
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	gap := historicalStackReplayGap(
		t, job, assemblyline.HistoricalPortableRendererV7,
	)
	point := queue.StationCallReplayPoint{Gap: gap, Call: replayTestCall(t, gap)}
	boundary, err := validateExactStationReplayPoint(point)
	if err != nil {
		t.Fatal(err)
	}
	if boundary.Prompt != gap.Prompt ||
		string(boundary.Job.Payload) != string(job.Payload) {
		t.Fatalf("historical V7 stack boundary=%+v", boundary)
	}
	artifact, err := replayExactStationArtifactForRenderer(
		boundary.Job, assemblyline.HistoricalPortableRendererV7,
		"STACK_CANDIDATE_1",
	)
	if err != nil || artifact.Kind != string(job.Kind) {
		t.Fatalf("historical V7 stack artifact=%+v error=%v", artifact, err)
	}
}

func historicalStackReplayGap(
	t *testing.T,
	job assemblyline.PortableJob,
	renderer string,
) queue.StationGapOpening {
	t.Helper()
	const prompt = "FROZEN_V1_STACK_CONSTRAINT_PROMPT"
	envelope, err := exactjson.Canonical(job)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := replayProjectionEnvelope(prompt, renderer)
	if err != nil {
		t.Fatal(err)
	}
	stationID, err := queue.StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := portableModelScope(job.Kind)
	if err != nil {
		t.Fatal(err)
	}
	maximum, err := queue.ExpectedPortableStationMaxOutputTokens(job, 8192)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := assemblyline.SemanticUncertaintyContractForPortableRenderer(
		renderer, job.Kind,
	)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := contract.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return queue.StationGapOpening{
		ID: 31, JobID: 7, Generation: 1, StepID: 2, StepAttempt: 1,
		WorkerID: "historical-stack-replay", GapID: job.ID,
		Station: stationID, Scope: scope, PortableSchema: job.Schema,
		WorkID: job.ID, WorkKind: string(job.Kind),
		PortablePayload: string(job.Payload), PortablePayloadSHA256: replaySHA256(string(job.Payload)),
		PortableEnvelope: string(envelope), PortableEnvelopeSHA256: replaySHA256(string(envelope)),
		RendererVersion: renderer, Prompt: prompt,
		ProjectionEnvelope: string(projection), ProjectionSHA256: replaySHA256(string(projection)),
		SemanticUncertaintyContract: contract, SemanticUncertaintyContractSHA256: digest,
		ContextTokens: 8192, MaxOutputTokens: maximum,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
}

func historicalPortableJobID(job assemblyline.PortableJob) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(job.Schema))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(job.Kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(job.Payload)
	return hex.EncodeToString(hash.Sum(nil))
}
