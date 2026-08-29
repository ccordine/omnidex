package worker

import (
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/queue"
)

func exactCurrentContractGap(
	point queue.StationCallReplayPoint,
	job assemblyline.PortableJob,
) (queue.StationGapOpening, llmResponseContract, error) {
	var zero queue.StationGapOpening
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	contract, err := llmResponseContractForPortableJob(job)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	envelope, err := exactjson.Canonical(job)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	projection, err := exactjson.Canonical(struct {
		Prompt   string `json:"prompt"`
		Renderer string `json:"renderer"`
	}{prompt, assemblyline.PortableRendererV1})
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	stationID, err := queue.StationForPortableJob(job)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	scope, err := portableModelScope(job.Kind)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	maxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(
		job, point.Gap.ContextTokens,
	)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	semanticUncertainty, err := assemblyline.SemanticUncertaintyContractForWorkKind(job.Kind)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	semanticUncertaintySHA256, err := semanticUncertainty.Digest()
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	gap := queue.StationGapOpening{
		ID: point.Gap.ID, JobID: point.Gap.JobID, Generation: point.Gap.Generation,
		StepID: point.Gap.StepID, StepAttempt: point.Gap.StepAttempt, WorkerID: point.Gap.WorkerID,
		GapID: job.ID, Station: stationID, Scope: scope,
		PortableSchema: job.Schema, WorkID: job.ID, WorkKind: string(job.Kind),
		PortablePayload: string(job.Payload), PortablePayloadSHA256: replaySHA256(string(job.Payload)),
		PortableEnvelope: string(envelope), PortableEnvelopeSHA256: replaySHA256(string(envelope)),
		RendererVersion: assemblyline.PortableRendererV1, Prompt: prompt,
		ProjectionEnvelope:                string(projection),
		ProjectionSHA256:                  replaySHA256(string(projection)),
		SemanticUncertaintyContract:       semanticUncertainty,
		SemanticUncertaintyContractSHA256: semanticUncertaintySHA256,
		ContextTokens:                     point.Gap.ContextTokens,
		MaxOutputTokens:                   maxOutputTokens, OutputLimitMode: contract.OutputLimitMode,
	}
	return gap, contract, nil
}
