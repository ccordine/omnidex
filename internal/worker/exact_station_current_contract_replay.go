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
// model-visible station prompt/schema from one historical opening while using
// the checked-in transport contract. It refuses renderer drift and performs no
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
	scope := fmt.Sprintf("station-current-contract:%d:%s", point.Call.ID, boundary.Job.ID)
	return replayCurrentPortableStation(ctx, client, point, boundary.Job, modelName, scope)
}

func replayCurrentPortableStation(
	ctx context.Context,
	client llm.ExactStationClient,
	point queue.StationCallReplayPoint,
	job assemblyline.PortableJob,
	modelName string,
	discoveryScope string,
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
	gap, contract, err := exactConvergenceGap(point, job)
	if err != nil {
		return result, err
	}
	_, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return result, fmt.Errorf("render current-contract station schema: %w", err)
	}
	selection := llm.ProviderIdentitySelection{
		Model:              result.Model,
		NativeContextLimit: gap.ContextTokens,
	}
	if err := validateExactStationStaticCall(gap.Prompt, schema, contract, selection); err != nil {
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
	prepared, err := prepareExactStationCall(gap, contract, result.Model, expected)
	if err != nil {
		return result, fmt.Errorf("prepare current-contract station replay request: %w", err)
	}
	request, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return result, fmt.Errorf("render current-contract station replay request: %w", err)
	}
	result.ExpectedIdentity = expected
	result.PreparedRequest = string(request)
	result.PreparedRequestSHA256 = replaySHA256(result.PreparedRequest)
	return executeExactStationReplayPrepared(ctx, client, result, job, gap.ContextTokens, prepared)
}
