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

func TestStationReplayValidationRejectsUnregisteredKindsDirectly(t *testing.T) {
	for _, kind := range []assemblyline.WorkKind{
		"conversation_context_selection",
		"memory_context_selection",
		"roleplay_narrative_continuity",
		"application_service_endpoint_contract",
		"application_service_deployment_intent",
		"application_context_needs",
		"application_intent",
		"application_job_specification",
		"application_service_state_interface",
		"repository_requirements",
		"repository_change_surface",
		"repository_search_term",
		"repository_search_anchor_coverage",
		"repository_search_anchor",
		"context_search_terms",
		"context_search_term_coverage",
		"context_search_term",
		"context_relevance",
		"repository_evidence_relevance",
		"repository_grounded_review",
		"repository_grounded_issue_detail",
		"repository_grounded_issue_kind",
		"repository_grounded_correction",
		"roleplay_canon_extraction",
		"roleplay_grounded_response",
		"grounded_answer",
		"database_schema_selection",
		"database_query_intent",
		"web_search_terms",
		"web_relevance",
		"web_grounded_synthesis",
		"web_grounded_synthesis_correction",
		"web_claim_evidence_review",
		"web_review_claim_coverage",
		"web_review_claim",
		"web_review_claim_verdict",
		"web_review_issue_evidence_relation",
		"web_review_issue_detail",
	} {
		t.Run(string(kind), func(t *testing.T) {
			retired := assemblyline.PortableJob{
				Schema: assemblyline.PortableJobSchemaV2, Kind: kind, Payload: json.RawMessage(`{}`),
			}
			if err := retired.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported") {
				t.Fatalf("direct unregistered validation error=%v", err)
			}
		})
	}
}

func TestValidateExactStationReplayPointPreservesPortableBoundary(t *testing.T) {
	job, err := assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: "function Repair(value: string): string",
		CurrentDeclaration: "function Repair(value: string): string { return value; }",
		RepairGuidance:     "Return the repaired value.",
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
		loaded.Prompt != gap.Prompt {
		t.Fatalf("replay boundary=%#v job=%#v", loaded, job)
	}
	stale := queue.StationCallReplayPoint{Call: call, Gap: gap}
	stale.Gap.Prompt = "stale renderer output"
	projection, err := replayProjectionEnvelope(
		stale.Gap.Prompt, stale.Gap.RendererVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	stale.Gap.ProjectionEnvelope = string(projection)
	stale.Gap.ProjectionSHA256 = replaySHA256(string(projection))
	if _, err := validateExactStationReplayPoint(stale); err == nil ||
		!strings.Contains(err.Error(), "differs from the current portable renderer") {
		t.Fatalf("stale renderer replay error=%v", err)
	}

	for name, mutate := range map[string]func(*queue.StationCallReplayPoint){
		"non-current renderer": func(point *queue.StationCallReplayPoint) {
			point.Gap.RendererVersion = "unsupported-renderer"
		},
		"stored prompt drift": func(point *queue.StationCallReplayPoint) {
			point.Gap.Prompt += "\nunauthorized"
		},
		"call gap mismatch": func(point *queue.StationCallReplayPoint) {
			point.Call.GapOpeningID++
		},
		"stored model input drift": func(point *queue.StationCallReplayPoint) {
			point.Call.ModelInput += "\nunauthorized"
		},
		"scope drift": func(point *queue.StationCallReplayPoint) {
			point.Gap.Scope = "portable_semantic_worker"
		},
		"semantic uncertainty drift": func(point *queue.StationCallReplayPoint) {
			point.Gap.SemanticUncertaintyContract.ExactQuestion = "Which forged value?"
		},
		"semantic uncertainty digest drift": func(point *queue.StationCallReplayPoint) {
			point.Gap.SemanticUncertaintyContractSHA256 = strings.Repeat("0", 64)
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

func TestValidateExactStationReplayPointBindsRawSingleLineChatMLBoundary(t *testing.T) {
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{
			UserRequest: "Describe the surface of a local reporting utility.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	gap := replayTestGap(t, job)
	call := replayTestCall(t, gap)
	if !strings.HasPrefix(call.ModelInput, llm.ExactPreparedRawChatUserPrefixV1) ||
		!strings.HasSuffix(call.ModelInput, llm.ExactPreparedRawChatAssistantBoundaryV1) {
		t.Fatalf("raw single-line replay input lacks its ChatML boundary: %q", call.ModelInput)
	}
	if _, err := validateExactStationReplayPoint(queue.StationCallReplayPoint{
		Call: call, Gap: gap,
	}); err != nil {
		t.Fatalf("raw single-line replay identity was rejected: %v", err)
	}

	call.ModelInput = strings.TrimPrefix(
		call.ModelInput, llm.ExactPreparedRawChatUserPrefixV1,
	)
	call.ModelInputBytes = len(call.ModelInput)
	call.ModelInputSHA256 = replaySHA256(call.ModelInput)
	if _, err := validateExactStationReplayPoint(queue.StationCallReplayPoint{
		Call: call, Gap: gap,
	}); err == nil {
		t.Fatal("raw single-line replay accepted an input without its code-owned ChatML boundary")
	}
}

func replayTestGap(t *testing.T, job assemblyline.PortableJob) queue.StationGapOpening {
	t.Helper()
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := exactjson.Canonical(job)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := replayTestProjection(prompt)
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
	maxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(job, 8192)
	if err != nil {
		t.Fatal(err)
	}
	gap := queue.StationGapOpening{
		ID: 17, JobID: 3, Generation: 1, StepID: 2, StepAttempt: 1, WorkerID: "replay-test",
		GapID: job.ID, Station: stationID, Scope: scope,
		PortableSchema: job.Schema, WorkID: job.ID, WorkKind: string(job.Kind),
		PortablePayload: string(job.Payload), PortablePayloadSHA256: replaySHA256(string(job.Payload)),
		PortableEnvelope: string(envelope), PortableEnvelopeSHA256: replaySHA256(string(envelope)),
		RendererVersion: assemblyline.PortableRendererV1, Prompt: prompt,
		ProjectionEnvelope: string(projection), ProjectionSHA256: replaySHA256(string(projection)),
		ContextTokens: 8192, MaxOutputTokens: maxOutputTokens,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
	bindTestGapSemanticUncertainty(t, &gap)
	return gap
}

func replayTestProjection(prompt string) ([]byte, error) {
	return exactjson.Canonical(struct {
		Prompt   string `json:"prompt"`
		Renderer string `json:"renderer"`
	}{prompt, assemblyline.PortableRendererV1})
}

func replayTestCall(t *testing.T, gap queue.StationGapOpening) queue.StationCallOpening {
	t.Helper()
	var persistedJob assemblyline.PortableJob
	if err := json.Unmarshal([]byte(gap.PortableEnvelope), &persistedJob); err != nil {
		t.Fatal(err)
	}
	job := assemblyline.PortableJob{
		Schema: gap.PortableSchema, ID: gap.WorkID, Kind: assemblyline.WorkKind(gap.WorkKind),
		Payload: json.RawMessage(gap.PortablePayload), SourceProjection: persistedJob.SourceProjection,
	}
	contract, err := llmResponseContractForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	expected := llm.ProviderIdentityExpectation{
		Backend: llm.ExactPreparedProviderBackend, BackendVersion: llm.ExactPreparedProviderVersion,
		Model: "qwen3.5:9b-q4_K_M", Digest: strings.Repeat("a", 64), Quantization: "Q4_K_M",
		NativeContextLimit: gap.ContextTokens, TokenizerProfile: llm.ExactPreparedTokenizerProfile,
	}
	stop, err := queue.ExpectedStationCallStopSequence(gap, expected)
	if err != nil {
		t.Fatal(err)
	}
	prepared := llm.PreparedModel{
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: gap.Prompt, PromptHint: contract.PromptHint,
		ContextTokens:               gap.ContextTokens,
		RawTextStopSequence:         stop,
		ProviderIdentityExpectation: &expected,
	}
	input, err := llm.ExactPreparedRequestModelInput(prepared)
	if err != nil {
		t.Fatal(err)
	}
	expectation, err := exactjson.Canonical(expected)
	if err != nil {
		t.Fatal(err)
	}
	return queue.StationCallOpening{
		ID: 19, GapOpeningID: gap.ID, JobID: gap.JobID, Generation: gap.Generation,
		StepID: gap.StepID, StepAttempt: gap.StepAttempt, WorkerID: gap.WorkerID,
		GapID: gap.GapID, Protocol: string(llm.ExactPreparedProtocolRawTextV2),
		TokenizerProfile: expected.TokenizerProfile, Expectation: expectation,
		ExpectationSHA256: replaySHA256(string(expectation)),
		Model:             expected.Model, ContextTokens: gap.ContextTokens,
		MaxInputTokens: gap.ContextTokens, MaxOutputTokens: gap.MaxOutputTokens,
		OutputLimitMode: gap.OutputLimitMode, ModelInput: input,
		ModelInputSHA256: replaySHA256(input), ModelInputBytes: len(input),
		ModelInputTokenCeiling: gap.ContextTokens,
	}
}
