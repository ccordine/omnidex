package queue

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

func validateStationGapOpening(record StationGapOpenRecord) (StationGapOpening, error) {
	if err := record.Job.Validate(); err != nil {
		return StationGapOpening{}, fmt.Errorf("station gap requires one validated PortableJob: %w", err)
	}
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return StationGapOpening{}, fmt.Errorf("station gap authority: %w", err)
	}
	if err := record.Station.Validate(); err != nil {
		return StationGapOpening{}, err
	}
	expectedStation, err := stationForPortableJob(record.Job)
	if err != nil {
		return StationGapOpening{}, err
	}
	if record.Station != expectedStation {
		return StationGapOpening{}, fmt.Errorf("station gap station %q does not own portable work kind %q", record.Station, record.Job.Kind)
	}
	prompt, err := assemblyline.RenderPortableJob(record.Job)
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("render exact station gap projection: %w", err)
	}
	if strings.TrimSpace(prompt) == "" {
		return StationGapOpening{}, fmt.Errorf("station gap prompt must contain exact non-empty text")
	}
	profilePolicy, err := PortableJobProviderIdentityProfilePolicy(record.Job)
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("classify station context policy: %w", err)
	}
	if err := validateStationOutputLimitAuthority(
		record.OutputLimitMode, record.ContextTokens, record.MaxOutputTokens, profilePolicy,
	); err != nil {
		return StationGapOpening{}, fmt.Errorf("station gap output authority: %w", err)
	}
	if record.OutputLimitMode == llm.ExactPreparedOutputLimitNatural {
		expectedMaxOutputTokens, err := ExpectedPortableStationMaxOutputTokens(
			record.Job, record.ContextTokens,
		)
		if err != nil {
			return StationGapOpening{}, fmt.Errorf("station gap output authority: %w", err)
		}
		if record.MaxOutputTokens != expectedMaxOutputTokens {
			return StationGapOpening{}, fmt.Errorf(
				"station gap natural output ceiling=%d differs from code-derived ceiling=%d",
				record.MaxOutputTokens, expectedMaxOutputTokens,
			)
		}
	}
	modelInput, err := llm.ExactPreparedModelInput(prompt, llm.MinimalGeneratePrompt)
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("station gap model input: %w", err)
	}
	if err := validateStationGapModelInputAuthority(record, modelInput); err != nil {
		return StationGapOpening{}, fmt.Errorf("station gap model input: %w", err)
	}
	scope, err := stationGapScope(record.Job.Kind)
	if err != nil {
		return StationGapOpening{}, err
	}
	if record.OutputLimitMode != llm.ExactPreparedOutputLimitNatural {
		return StationGapOpening{}, fmt.Errorf(
			"station gap scope %q rejects output-limit mode %q",
			scope, record.OutputLimitMode,
		)
	}
	schema, err := canonicalStationGapSchema()
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("canonicalize station gap response schema: %w", err)
	}
	portableEnvelope, err := exactjson.Canonical(record.Job)
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("canonicalize station gap PortableJob: %w", err)
	}
	if len(portableEnvelope) > maxStationRequestResourceBytes {
		return StationGapOpening{}, fmt.Errorf(
			"station gap portable envelope exceeds coarse %d-byte request resource ceiling",
			maxStationRequestResourceBytes,
		)
	}
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{prompt, assemblyline.PortableRendererV4, schema})
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("canonicalize station gap projection: %w", err)
	}
	if len(projection) > maxStationRequestResourceBytes {
		return StationGapOpening{}, fmt.Errorf(
			"station gap projection exceeds coarse %d-byte request resource ceiling",
			maxStationRequestResourceBytes,
		)
	}
	return StationGapOpening{
		JobID: record.Authority.JobID, Generation: record.Authority.Generation,
		StepID: record.Authority.StepID, StepAttempt: record.Authority.Attempt,
		WorkerID: record.Authority.WorkerID, GapID: record.Job.ID,
		Station: record.Station, Scope: scope, PortableSchema: record.Job.Schema,
		WorkID: record.Job.ID, WorkKind: string(record.Job.Kind),
		PortablePayload: string(record.Job.Payload), PortablePayloadSHA256: stationGapSHA256(string(record.Job.Payload)),
		PortableEnvelope: string(portableEnvelope), PortableEnvelopeSHA256: stationGapSHA256(string(portableEnvelope)),
		RendererVersion: assemblyline.PortableRendererV4, Prompt: prompt,
		ResponseSchema: append(json.RawMessage(nil), schema...), ProjectionEnvelope: string(projection),
		ProjectionSHA256: stationGapSHA256(string(projection)), ContextTokens: record.ContextTokens,
		MaxOutputTokens: record.MaxOutputTokens, OutputLimitMode: record.OutputLimitMode,
	}, nil
}

func validateStationGapModelInputAuthority(record StationGapOpenRecord, modelInput string) error {
	if record.OutputLimitMode == llm.ExactPreparedOutputLimitNatural {
		return llm.ValidateExactPreparedNaturalInputAuthority(record.ContextTokens, modelInput)
	}
	return llm.ValidateExactPreparedInputAuthority(
		record.ContextTokens,
		record.ContextTokens-record.MaxOutputTokens,
		record.MaxOutputTokens,
		modelInput,
	)
}

func validateStationOutputLimitAuthority(
	mode llm.ExactPreparedOutputLimitMode,
	contextTokens int,
	maxOutputTokens int,
	profilePolicy llm.ProviderIdentityProfilePolicy,
) error {
	if err := profilePolicy.Validate(); err != nil {
		return err
	}
	contextErr := llm.ValidateInferenceContextTokens(contextTokens)
	if profilePolicy != "" {
		contextErr = llm.ValidateRoleplayCompletionContextTokens(contextTokens)
	}
	if contextErr != nil {
		return contextErr
	}
	if err := mode.Validate(); err != nil {
		return err
	}
	switch mode {
	case llm.ExactPreparedOutputLimitExplicit:
		if maxOutputTokens < 1 || maxOutputTokens > 16384 || maxOutputTokens >= contextTokens {
			return fmt.Errorf("explicit output ceiling must be within 1..16384 tokens and leave positive input authority")
		}
	case llm.ExactPreparedOutputLimitNatural:
		if maxOutputTokens < 1 || maxOutputTokens > contextTokens {
			return fmt.Errorf("natural output authority must be positive and within the native context")
		}
	}
	return nil
}

func stationGapScope(kind assemblyline.WorkKind) (string, error) {
	return assemblyline.PortableWorkerScopeForWorkKind(kind)
}

func validateStationGapTerminal(record StationGapTerminalRecord) error {
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return fmt.Errorf("station gap terminal authority: %w", err)
	}
	if record.OpeningID < 1 || len(record.GapID) != 64 || !llmEvidenceLowerHex(record.GapID) {
		return fmt.Errorf("station gap terminal requires exact opening and gap identities")
	}
	if len(record.Response) > maxStationGapResponseBytes || len(record.Error) > maxStationGapErrorBytes {
		return fmt.Errorf("station gap terminal exceeds bounded response or error size")
	}
	switch record.Status {
	case StationGapResolved:
		if strings.TrimSpace(record.Response) == "" || record.Error != "" || record.Projection == nil {
			return fmt.Errorf("resolved station gap requires one projected response and no error")
		}
		if err := validateStationGapSourceProjection(*record.Projection, len(record.Response)); err != nil {
			return fmt.Errorf("resolved station gap projection: %w", err)
		}
	case StationGapFailed:
		if record.Response != "" || record.Projection != nil || strings.TrimSpace(record.Error) == "" {
			return fmt.Errorf("failed station gap requires one error and no response projection")
		}
	default:
		return fmt.Errorf("station gap terminal status %q is unsupported", record.Status)
	}
	return nil
}

func validateStationGapSourceProjection(
	projection StationGapSourceProjection,
	responseBytes int,
) error {
	if projection.Kind != StationGapProjectionExactResponse &&
		projection.Kind != StationGapProjectionSourceDeclaration &&
		projection.Kind != StationGapProjectionTypeScriptFunction {
		return fmt.Errorf("kind %q is not registered", projection.Kind)
	}
	if !llmEvidenceLowerHex(projection.CallReceiptSHA256) || len(projection.CallReceiptSHA256) != 64 ||
		!llmEvidenceLowerHex(projection.SourceResponseSHA256) || len(projection.SourceResponseSHA256) != 64 {
		return fmt.Errorf("requires exact receipt and source response identities")
	}
	if projection.StartByte < 0 || projection.EndByte <= projection.StartByte ||
		projection.EndByte-projection.StartByte != responseBytes {
		return fmt.Errorf("source span does not match projected response bytes")
	}
	if projection.Kind == StationGapProjectionExactResponse && projection.StartByte != 0 {
		return fmt.Errorf("exact response projection must begin at byte zero")
	}
	return nil
}
