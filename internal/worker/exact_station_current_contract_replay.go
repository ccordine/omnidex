package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

// ReplayStationWithCurrentContract preserves the immutable portable job and
// model-visible station prompt from one raw opening while using the checked-in
// transport contract. It refuses renderer drift and performs no
// queue, historical-job, or workspace writes.
func ReplayStationWithCurrentContract(
	ctx context.Context,
	client llm.ExactStationClient,
	point queue.StationCallReplayPoint,
	modelName string,
) (ExactStationReplay, error) {
	boundary, err := validateCurrentContractStationReplayPoint(point)
	if err != nil {
		return ExactStationReplay{}, err
	}
	scope := fmt.Sprintf(
		"station-current-contract:%d:%s:semantic-uncertainty:%s",
		point.Call.ID, boundary.Job.ID, point.Gap.SemanticUncertaintyContractSHA256,
	)
	return replayCurrentPortableStation(ctx, client, point, boundary.Job, modelName, scope, nil)
}

func replayCurrentPortableStation(
	ctx context.Context,
	client llm.ExactStationClient,
	point queue.StationCallReplayPoint,
	job assemblyline.PortableJob,
	modelName string,
	discoveryScope string,
	temperature *llm.ExactPreparedTemperature,
) (ExactStationReplay, error) {
	result := ExactStationReplay{
		SourceOpeningID:    point.Call.ID,
		SourceGapOpeningID: point.Gap.ID,
		Job:                job,
		Model:              strings.TrimSpace(modelName),
	}
	if ctx == nil || client == nil || result.Model == "" || strings.TrimSpace(discoveryScope) == "" {
		return result, fmt.Errorf("current-contract station replay requires context, provider, model, and discovery scope")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	gap, contract, err := exactCurrentContractGap(point, job)
	if err != nil {
		return result, err
	}
	_, err = assemblyline.RenderPortableJob(job)
	if err != nil {
		return result, fmt.Errorf("render current-contract station transport: %w", err)
	}
	selection, err := providerSelectionForPortableJob(job, result.Model, gap.ContextTokens)
	if err != nil {
		return result, err
	}
	if err := validateExactStationStaticCall(gap.Prompt, contract, selection); err != nil {
		return result, err
	}
	if err := client.RequireExactPreparedContract(); err != nil {
		return result, fmt.Errorf("station replay provider: %w", err)
	}
	observed, err := llm.RequireDiscoveredProviderIdentityEvidence(
		ctx, client, selection, discoveryScope,
	)
	if err != nil {
		return result, fmt.Errorf("discover current-contract station replay provider: %w", err)
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(observed.Evidence, selection)
	if err != nil {
		return result, fmt.Errorf("derive current-contract station replay identity: %w", err)
	}
	prepared, err := prepareExactStationCall(gap, contract, result.Model, expected, temperature)
	if err != nil {
		return result, fmt.Errorf("prepare current-contract station replay request: %w", err)
	}
	request, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return result, fmt.Errorf("render current-contract station replay request: %w", err)
	}
	result.ExpectedIdentity = expected
	result.Temperature = prepared.Temperature
	result.PreparedRequest = string(request)
	result.PreparedRequestSHA256 = replaySHA256(result.PreparedRequest)
	return executeExactStationReplayPrepared(ctx, client, result, job, gap.ContextTokens, prepared)
}
