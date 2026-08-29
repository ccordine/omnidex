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
	if gap.RendererVersion != assemblyline.PortableRendererV1 {
		return boundary, fmt.Errorf("station replay renderer %q is not the current portable renderer", gap.RendererVersion)
	}
	if err := queue.ValidateStationGapSemanticUncertainty(gap); err != nil {
		return boundary, fmt.Errorf("station replay semantic uncertainty: %w", err)
	}
	var persistedJob assemblyline.PortableJob
	if err := exactjson.ValidateObject(
		[]byte(gap.PortableEnvelope), persistedJob, "station replay portable envelope",
	); err != nil {
		return boundary, fmt.Errorf("validate station replay portable envelope: %w", err)
	}
	if err := json.Unmarshal([]byte(gap.PortableEnvelope), &persistedJob); err != nil {
		return boundary, fmt.Errorf("decode station replay portable envelope: %w", err)
	}
	boundary.Job = assemblyline.PortableJob{
		Schema: gap.PortableSchema, ID: gap.WorkID, Kind: assemblyline.WorkKind(gap.WorkKind),
		Payload:          append(json.RawMessage(nil), gap.PortablePayload...),
		SourceProjection: persistedJob.SourceProjection,
	}
	if err := assemblyline.ValidatePortableJobForRenderer(
		boundary.Job, gap.RendererVersion,
	); err != nil {
		return boundary, fmt.Errorf("validate station replay portable job: %w", err)
	}
	if boundary.Job.Kind == assemblyline.WorkFragmentGenerationReplacement {
		if gap.OriginGapOpeningID < 1 || gap.OriginCallReceiptID < 1 {
			return boundary, fmt.Errorf(
				"station replay fragment generation replacement lacks exact persisted origin authority",
			)
		}
	} else if gap.OriginGapOpeningID != 0 || gap.OriginCallReceiptID != 0 {
		return boundary, fmt.Errorf(
			"station replay non-replacement work claims fragment generation origin authority",
		)
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
	projection, err := replayProjectionEnvelope(gap.Prompt, gap.RendererVersion)
	if err != nil || string(projection) != gap.ProjectionEnvelope ||
		replaySHA256(string(projection)) != gap.ProjectionSHA256 {
		return boundary, fmt.Errorf("station replay projection differs from its stored identity")
	}
	contract, err := llmResponseContractForPortableJob(boundary.Job)
	if err != nil {
		return boundary, err
	}
	boundary.Prompt, boundary.Contract = gap.Prompt, contract
	return boundary, nil
}

func validateCurrentContractStationReplayPoint(
	point queue.StationCallReplayPoint,
) (exactStationReplayBoundary, error) {
	boundary, err := loadStationReplayPortableBoundary(point)
	if err != nil {
		return boundary, err
	}
	if point.Gap.RendererVersion != assemblyline.PortableRendererV1 {
		return boundary, fmt.Errorf(
			"current-contract replay requires renderer %q, received %q",
			assemblyline.PortableRendererV1, point.Gap.RendererVersion,
		)
	}
	prompt, err := assemblyline.RenderPortableJob(boundary.Job)
	if err != nil {
		return boundary, fmt.Errorf("render current station replay contract: %w", err)
	}
	if prompt != point.Gap.Prompt {
		return boundary, fmt.Errorf("current station renderer differs from the frozen model-visible packet")
	}
	call, gap := point.Call, point.Gap
	expectedMaxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(
		boundary.Job, gap.ContextTokens,
	)
	if err != nil {
		return boundary, err
	}
	if call.ContextTokens != gap.ContextTokens || call.MaxOutputTokens != gap.MaxOutputTokens ||
		gap.MaxOutputTokens != expectedMaxOutputTokens ||
		call.OutputLimitMode != gap.OutputLimitMode || call.ModelInputBytes != len(call.ModelInput) ||
		replaySHA256(call.ModelInput) != call.ModelInputSHA256 {
		return boundary, fmt.Errorf("stored station call differs from its transport authority")
	}
	return boundary, nil
}

func replayProjectionEnvelope(prompt, renderer string) ([]byte, error) {
	return exactjson.Canonical(struct {
		Prompt   string `json:"prompt"`
		Renderer string `json:"renderer"`
	}{prompt, renderer})
}
