package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/queue"
)

func loadStationReplayPortableBoundary(
	point queue.StationCallReplayPoint,
) (exactStationReplayBoundary, error) {
	var boundary exactStationReplayBoundary
	call, gap := point.Call, point.Gap
	if call.ID < 1 || gap.ID < 1 || call.GapOpeningID != gap.ID || call.JobID != gap.JobID ||
		call.Generation != gap.Generation || call.StepID != gap.StepID ||
		call.StepAttempt != gap.StepAttempt || call.WorkerID != gap.WorkerID || call.GapID != gap.GapID {
		return boundary, fmt.Errorf("station replay point does not preserve one exact call and gap authority")
	}
	if gap.RendererVersion != assemblyline.PortableRendererV3 {
		return boundary, fmt.Errorf("station replay renderer %q is not current", gap.RendererVersion)
	}
	if err := exactjson.ValidateObject(
		[]byte(gap.PortableEnvelope), assemblyline.PortableJob{}, "station replay portable envelope",
	); err != nil {
		return boundary, fmt.Errorf("validate station replay portable envelope: %w", err)
	}
	boundary.Job = assemblyline.PortableJob{
		Schema: gap.PortableSchema, ID: gap.WorkID, Kind: assemblyline.WorkKind(gap.WorkKind),
		Payload: append(json.RawMessage(nil), gap.PortablePayload...),
	}
	if err := rejectRetiredStationReplayJob(boundary.Job); err != nil {
		return boundary, err
	}
	if err := boundary.Job.Validate(); err != nil {
		return boundary, fmt.Errorf("validate station replay portable job: %w", err)
	}
	envelope, err := exactjson.Canonical(boundary.Job)
	if err != nil || string(envelope) != gap.PortableEnvelope ||
		replaySHA256(string(envelope)) != gap.PortableEnvelopeSHA256 {
		return boundary, fmt.Errorf("station replay portable envelope differs from its stored identity")
	}
	if boundary.Job.Schema != gap.PortableSchema || boundary.Job.ID != gap.WorkID ||
		string(boundary.Job.Kind) != gap.WorkKind || string(boundary.Job.Payload) != gap.PortablePayload ||
		replaySHA256(string(boundary.Job.Payload)) != gap.PortablePayloadSHA256 {
		return boundary, fmt.Errorf("station replay portable fields differ from their stored identity")
	}
	if strings.TrimSpace(gap.Prompt) == "" {
		return boundary, fmt.Errorf("station replay stored prompt is empty")
	}
	var schema map[string]any
	if string(gap.ResponseSchema) != "null" {
		if err := json.Unmarshal(gap.ResponseSchema, &schema); err != nil {
			return boundary, fmt.Errorf("decode station replay response schema: %w", err)
		}
	}
	schemaRaw, err := exactjson.Canonical(schema)
	if err != nil || string(schemaRaw) != string(gap.ResponseSchema) {
		return boundary, fmt.Errorf("station replay response schema differs from its stored canonical identity")
	}
	projection, err := replayProjectionEnvelope(gap.Prompt, schemaRaw)
	if err != nil || string(projection) != gap.ProjectionEnvelope ||
		replaySHA256(string(projection)) != gap.ProjectionSHA256 {
		return boundary, fmt.Errorf("station replay projection differs from its stored identity")
	}
	contract, err := llmResponseContractForPortableJob(boundary.Job, schema)
	if err != nil {
		return boundary, err
	}
	boundary.Prompt, boundary.Schema, boundary.Contract = gap.Prompt, schema, contract
	return boundary, nil
}

func rejectRetiredStationReplayJob(job assemblyline.PortableJob) error {
	switch job.Kind {
	case assemblyline.WorkKind("conversation_context_selection"),
		assemblyline.WorkKind("memory_context_selection"),
		assemblyline.WorkKind("roleplay_narrative_continuity"),
		assemblyline.WorkKind("application_service_endpoint_contract"),
		assemblyline.WorkKind("application_service_deployment_intent"):
		return fmt.Errorf("station replay rejects retired station work kind %q", job.Kind)
	case assemblyline.WorkResponseCorrection:
		var correction assemblyline.ResponseCorrectionInput
		if err := json.Unmarshal(job.Payload, &correction); err != nil {
			return fmt.Errorf("decode station replay correction authority: %w", err)
		}
		if correction.Original.Kind == assemblyline.WorkResponseCorrection {
			return fmt.Errorf("station replay rejects nested response correction authority")
		}
		if err := rejectRetiredStationReplayJob(correction.Original); err != nil {
			return err
		}
		if strings.TrimSpace(correction.RetainedCandidate) == "" &&
			correction.Original.Kind != assemblyline.WorkApplicationJobSpecification {
			return fmt.Errorf(
				"station replay rejects %s correction without one exact retained candidate",
				correction.Original.Kind,
			)
		}
		return nil
	default:
		return nil
	}
}

func validateCurrentContractStationReplayPoint(
	point queue.StationCallReplayPoint,
) (exactStationReplayBoundary, error) {
	boundary, err := loadStationReplayPortableBoundary(point)
	if err != nil {
		return boundary, err
	}
	prompt, schema, err := assemblyline.RenderPortableJob(boundary.Job)
	if err != nil {
		return boundary, fmt.Errorf("render current station replay contract: %w", err)
	}
	schemaRaw, err := exactjson.Canonical(schema)
	if err != nil || prompt != point.Gap.Prompt || string(schemaRaw) != string(point.Gap.ResponseSchema) {
		return boundary, fmt.Errorf("current station renderer differs from the frozen model-visible packet")
	}
	call, gap := point.Call, point.Gap
	if call.ContextTokens != gap.ContextTokens || call.MaxOutputTokens != gap.MaxOutputTokens ||
		call.OutputLimitMode != gap.OutputLimitMode || call.ModelInputBytes != len(call.ModelInput) ||
		replaySHA256(call.ModelInput) != call.ModelInputSHA256 {
		return boundary, fmt.Errorf("stored station call differs from its historical transport authority")
	}
	return boundary, nil
}

func replayProjectionEnvelope(prompt string, schema json.RawMessage) ([]byte, error) {
	return exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{prompt, assemblyline.PortableRendererV3, schema})
}
