package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

// ConvergeExactTypeScriptStation preserves one frozen correction's source,
// signature, declarations, and symbols while the current compiler derives the
// exact model failure for every iteration. It performs no queue,
// historical-job, or workspace writes.
func ConvergeExactTypeScriptStation(
	ctx context.Context,
	client llm.ExactStationClient,
	point queue.StationCallReplayPoint,
	modelName string,
) (ExactTypeScriptConvergence, error) {
	boundary, err := validateExactStationReplayPoint(point)
	if err != nil {
		return ExactTypeScriptConvergence{}, err
	}
	var input assemblyline.FragmentCorrectionInput
	if err := json.Unmarshal(boundary.Job.Payload, &input); err != nil {
		return ExactTypeScriptConvergence{}, fmt.Errorf("decode TypeScript convergence input: %w", err)
	}
	compiler, err := newExactTypeScriptReplayCompiler(ctx, input)
	if err != nil {
		return ExactTypeScriptConvergence{}, fmt.Errorf("create TypeScript convergence compiler: %w", err)
	}
	defer compiler.Close()
	runtime := exactTypeScriptConvergenceRuntime{
		verify: compiler.Verify,
		replay: func(callCtx context.Context, job assemblyline.PortableJob, iteration int) (ExactStationReplay, error) {
			return replayDerivedExactStation(callCtx, client, point, job, modelName, iteration)
		},
	}
	return convergeExactTypeScriptStationWithRuntime(ctx, point, modelName, runtime)
}

func replayDerivedExactStation(
	ctx context.Context,
	client llm.ExactStationClient,
	point queue.StationCallReplayPoint,
	job assemblyline.PortableJob,
	modelName string,
	iteration int,
) (ExactStationReplay, error) {
	if iteration < 1 {
		return ExactStationReplay{}, fmt.Errorf("derived station replay requires one positive iteration")
	}
	scope := fmt.Sprintf("station-convergence:%d:%d:%s", point.Call.ID, iteration, job.ID)
	return replayCurrentPortableStation(ctx, client, point, job, modelName, scope)
}

func exactConvergenceGap(
	point queue.StationCallReplayPoint,
	job assemblyline.PortableJob,
) (queue.StationGapOpening, llmResponseContract, error) {
	var zero queue.StationGapOpening
	prompt, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	contract, err := llmResponseContractForPortableJob(job, schema)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	schemaRaw, err := exactjson.Canonical(schema)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	envelope, err := exactjson.Canonical(job)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{prompt, assemblyline.PortableRendererV3, schemaRaw})
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	stationID, err := queue.StationForPortableJob(job)
	if err != nil {
		return zero, llmResponseContract{}, err
	}
	gap := queue.StationGapOpening{
		ID: point.Gap.ID, JobID: point.Gap.JobID, Generation: point.Gap.Generation,
		StepID: point.Gap.StepID, StepAttempt: point.Gap.StepAttempt, WorkerID: point.Gap.WorkerID,
		GapID: job.ID, Station: stationID, Scope: portableModelScope(schema),
		PortableSchema: job.Schema, WorkID: job.ID, WorkKind: string(job.Kind),
		PortablePayload: string(job.Payload), PortablePayloadSHA256: replaySHA256(string(job.Payload)),
		PortableEnvelope: string(envelope), PortableEnvelopeSHA256: replaySHA256(string(envelope)),
		RendererVersion: assemblyline.PortableRendererV3, Prompt: prompt,
		ResponseSchema: schemaRaw, ProjectionEnvelope: string(projection),
		ProjectionSHA256: replaySHA256(string(projection)), ContextTokens: point.Gap.ContextTokens,
		MaxOutputTokens: point.Gap.ContextTokens, OutputLimitMode: contract.OutputLimitMode,
	}
	return gap, contract, nil
}
