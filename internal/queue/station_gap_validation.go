package queue

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
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
	prompt, renderedSchema, err := assemblyline.RenderPortableJob(record.Job)
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("render exact station gap projection: %w", err)
	}
	if strings.TrimSpace(prompt) == "" || len(prompt) > maxStationGapPromptBytes {
		return StationGapOpening{}, fmt.Errorf("station gap prompt must contain 1..%d exact bytes", maxStationGapPromptBytes)
	}
	if record.ContextTokens < 1 || record.ContextTokens > 262144 ||
		record.MaxOutputTokens < 1 || record.MaxOutputTokens > 16384 {
		return StationGapOpening{}, fmt.Errorf("station gap requires bounded positive inference limits")
	}
	schema, err := canonicalStationGapSchema(renderedSchema)
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("canonicalize station gap response schema: %w", err)
	}
	if len(schema) > maxStationGapSchemaBytes {
		return StationGapOpening{}, fmt.Errorf("station gap response schema exceeds %d bytes", maxStationGapSchemaBytes)
	}
	portableEnvelope, err := exactjson.Canonical(record.Job)
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("canonicalize station gap PortableJob: %w", err)
	}
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{prompt, assemblyline.PortableRendererV1, schema})
	if err != nil {
		return StationGapOpening{}, fmt.Errorf("canonicalize station gap projection: %w", err)
	}
	if len(projection) > maxStationGapEnvelopeBytes {
		return StationGapOpening{}, fmt.Errorf("station gap projection exceeds %d bytes", maxStationGapEnvelopeBytes)
	}
	return StationGapOpening{
		JobID: record.Authority.JobID, Generation: record.Authority.Generation,
		StepID: record.Authority.StepID, StepAttempt: record.Authority.Attempt,
		WorkerID: record.Authority.WorkerID, GapID: record.Job.ID,
		Station: record.Station, Scope: stationGapScope(renderedSchema), PortableSchema: record.Job.Schema,
		WorkID: record.Job.ID, WorkKind: string(record.Job.Kind),
		PortablePayload: string(record.Job.Payload), PortablePayloadSHA256: stationGapSHA256(string(record.Job.Payload)),
		PortableEnvelope: string(portableEnvelope), PortableEnvelopeSHA256: stationGapSHA256(string(portableEnvelope)),
		RendererVersion: assemblyline.PortableRendererV1, Prompt: prompt,
		ResponseSchema: append(json.RawMessage(nil), schema...), ProjectionEnvelope: string(projection),
		ProjectionSHA256: stationGapSHA256(string(projection)), ContextTokens: record.ContextTokens,
		MaxOutputTokens: record.MaxOutputTokens,
	}, nil
}

func stationGapScope(schema map[string]any) string {
	if schema == nil {
		return "portable_fragment_worker"
	}
	return "portable_semantic_worker"
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
		if strings.TrimSpace(record.Response) == "" || record.Error != "" {
			return fmt.Errorf("resolved station gap requires one response and no error")
		}
	case StationGapFailed:
		if record.Response != "" || strings.TrimSpace(record.Error) == "" {
			return fmt.Errorf("failed station gap requires one error and no response")
		}
	default:
		return fmt.Errorf("station gap terminal status %q is unsupported", record.Status)
	}
	return nil
}
