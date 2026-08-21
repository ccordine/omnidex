package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestStationReplayRejectsRetiredContextKindsDirectlyAndThroughCorrection(t *testing.T) {
	for _, kind := range []assemblyline.WorkKind{
		"conversation_context_selection",
		"memory_context_selection",
		"roleplay_narrative_continuity",
	} {
		t.Run(string(kind), func(t *testing.T) {
			retired := assemblyline.PortableJob{Kind: kind}
			if err := rejectRetiredStationReplayJob(retired); err == nil ||
				!strings.Contains(err.Error(), "retired context work kind") {
				t.Fatalf("direct retired replay error=%v", err)
			}
			payload, err := json.Marshal(assemblyline.ResponseCorrectionInput{Original: retired})
			if err != nil {
				t.Fatal(err)
			}
			nested := assemblyline.PortableJob{
				Kind: assemblyline.WorkResponseCorrection, Payload: payload,
			}
			if err := rejectRetiredStationReplayJob(nested); err == nil ||
				!strings.Contains(err.Error(), "retired context work kind") {
				t.Fatalf("nested retired replay error=%v", err)
			}
		})
	}
}

func TestStationReplayRejectsGenericCorrectionWithoutRetainedCandidate(t *testing.T) {
	original, err := assemblyline.NewKnownArtifactTruthJob(assemblyline.KnownArtifactTruthInput{
		RequirementQuote: "The known semantic artifact must be absent.",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(assemblyline.ResponseCorrectionInput{
		Original: original, ValidationFailure: "truth is unsupported",
	})
	if err != nil {
		t.Fatal(err)
	}
	job := assemblyline.PortableJob{Kind: assemblyline.WorkResponseCorrection, Payload: payload}
	if err := rejectRetiredStationReplayJob(job); err == nil ||
		!strings.Contains(err.Error(), "without one exact retained candidate") {
		t.Fatalf("empty-retained replay error=%v", err)
	}
}

func TestValidateExactStationReplayPointPreservesFrozenPortableBoundary(t *testing.T) {
	job, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function Repair(value: string): string",
		CurrentDeclaration: "function Repair(value: string): string { return value; }",
		RequiredChange:     "Fix the observed local failure.",
		Diagnostic:         "[source]: error TS2304: Cannot find name 'value'.",
	})
	if err != nil {
		t.Fatal(err)
	}
	gap := replayTestGap(t, job)
	call := replayTestCall(t, gap)

	loaded, err := validateExactStationReplayPoint(queue.StationCallReplayPoint{
		Call: call,
		Gap:  gap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Job.ID != job.ID || string(loaded.Job.Payload) != string(job.Payload) ||
		loaded.Prompt != gap.Prompt || loaded.Schema != nil {
		t.Fatalf("replay boundary=%#v job=%#v", loaded, job)
	}

	t.Run("historical renderer drift retains the stored projection", func(t *testing.T) {
		point := queue.StationCallReplayPoint{Call: call, Gap: gap}
		point.Gap.Prompt = "FROZEN_STATION_PROMPT_V1"
		projection, err := replayTestProjection(point.Gap.Prompt, point.Gap.ResponseSchema)
		if err != nil {
			t.Fatal(err)
		}
		point.Gap.ProjectionEnvelope = string(projection)
		point.Gap.ProjectionSHA256 = replaySHA256(string(projection))
		input, err := llm.ExactPreparedModelInput(point.Gap.Prompt, llm.MinimalGeneratePrompt)
		if err != nil {
			t.Fatal(err)
		}
		point.Call.ModelInput, point.Call.ModelInputBytes = input, len(input)
		point.Call.ModelInputSHA256 = replaySHA256(input)

		loaded, err := validateExactStationReplayPoint(point)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Prompt != point.Gap.Prompt {
			t.Fatalf("replay prompt=%q, want stored frozen prompt %q", loaded.Prompt, point.Gap.Prompt)
		}
	})

	for name, mutate := range map[string]func(*queue.StationCallReplayPoint){
		"stored prompt drift": func(point *queue.StationCallReplayPoint) {
			point.Gap.Prompt += "\nunauthorized"
		},
		"call gap mismatch": func(point *queue.StationCallReplayPoint) {
			point.Call.GapOpeningID++
		},
		"stored model input drift": func(point *queue.StationCallReplayPoint) {
			point.Call.ModelInput += "\nunauthorized"
		},
		"response schema scope drift": func(point *queue.StationCallReplayPoint) {
			point.Gap.Scope = "portable_semantic_worker"
		},
	} {
		t.Run(name, func(t *testing.T) {
			point := queue.StationCallReplayPoint{Call: call, Gap: gap}
			mutate(&point)
			if _, err := validateExactStationReplayPoint(point); err == nil {
				t.Fatal("expected exact replay boundary rejection")
			}
		})
	}
}

func TestValidateCurrentContractStationReplayPointRetainsJobWithoutRetiredOutputCeiling(t *testing.T) {
	job, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function Repair(value: string): string",
		CurrentDeclaration: "function Repair(value: string): string { return value; }",
		RequiredChange:     "Fix the observed local failure.",
		Diagnostic:         "[source]: error TS2304: Cannot find name 'value'.",
	})
	if err != nil {
		t.Fatal(err)
	}
	gap := replayTestGap(t, job)
	call := replayTestCall(t, gap)
	gap.OutputLimitMode = llm.ExactPreparedOutputLimitExplicit
	gap.MaxOutputTokens = 1024
	call.OutputLimitMode = llm.ExactPreparedOutputLimitExplicit
	call.MaxOutputTokens = 1024
	point := queue.StationCallReplayPoint{Call: call, Gap: gap}
	if _, err := validateExactStationReplayPoint(point); err == nil {
		t.Fatal("exact replay silently changed retired output authority")
	}
	loaded, err := validateCurrentContractStationReplayPoint(point)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Job.ID != job.ID || loaded.Prompt != gap.Prompt || loaded.Contract.OutputLimitMode != llm.ExactPreparedOutputLimitNatural {
		t.Fatalf("current-contract replay boundary=%+v", loaded)
	}

	point.Gap.Prompt += "\nchanged"
	if _, err := validateCurrentContractStationReplayPoint(point); err == nil {
		t.Fatal("current-contract replay accepted a prompt different from the current renderer")
	}
}

func replayTestGap(t *testing.T, job assemblyline.PortableJob) queue.StationGapOpening {
	t.Helper()
	prompt, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	schemaRaw, err := exactjson.Canonical(schema)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := exactjson.Canonical(job)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := replayTestProjection(prompt, schemaRaw)
	if err != nil {
		t.Fatal(err)
	}
	stationID, err := queue.StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	return queue.StationGapOpening{
		ID: 17, JobID: 3, Generation: 1, StepID: 2, StepAttempt: 1, WorkerID: "replay-test",
		GapID: job.ID, Station: stationID, Scope: "portable_fragment_worker",
		PortableSchema: job.Schema, WorkID: job.ID, WorkKind: string(job.Kind),
		PortablePayload: string(job.Payload), PortablePayloadSHA256: replaySHA256(string(job.Payload)),
		PortableEnvelope: string(envelope), PortableEnvelopeSHA256: replaySHA256(string(envelope)),
		RendererVersion: assemblyline.PortableRendererV3, Prompt: prompt,
		ResponseSchema: schemaRaw, ProjectionEnvelope: string(projection), ProjectionSHA256: replaySHA256(string(projection)),
		ContextTokens: 8192, MaxOutputTokens: 8192,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
}

func replayTestProjection(prompt string, schema json.RawMessage) ([]byte, error) {
	return exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{prompt, assemblyline.PortableRendererV3, schema})
}

func replayTestCall(t *testing.T, gap queue.StationGapOpening) queue.StationCallOpening {
	t.Helper()
	input, err := llm.ExactPreparedModelInput(gap.Prompt, llm.MinimalGeneratePrompt)
	if err != nil {
		t.Fatal(err)
	}
	return queue.StationCallOpening{
		ID: 19, GapOpeningID: gap.ID, JobID: gap.JobID, Generation: gap.Generation,
		StepID: gap.StepID, StepAttempt: gap.StepAttempt, WorkerID: gap.WorkerID,
		GapID: gap.GapID, Protocol: string(llm.ExactPreparedProtocolRawTextV1),
		Model: "qwen3.5:9b-q4_K_M", ContextTokens: gap.ContextTokens,
		MaxInputTokens: gap.ContextTokens, MaxOutputTokens: gap.MaxOutputTokens,
		OutputLimitMode: gap.OutputLimitMode, ModelInput: input,
		ModelInputSHA256: replaySHA256(input), ModelInputBytes: len(input),
		ModelInputTokenCeiling: gap.ContextTokens,
	}
}
