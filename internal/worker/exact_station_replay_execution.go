package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func executeExactStationReplayPrepared(
	ctx context.Context,
	client llm.ExactStationClient,
	result ExactStationReplay,
	job assemblyline.PortableJob,
	contextTokens int,
	prepared llm.PreparedModel,
) (ExactStationReplay, error) {
	started := time.Now()
	generation, generationErr := client.GeneratePreparedExact(ctx, prepared)
	result.WallDuration = time.Since(started)
	owned, ownershipErr := llm.OwnBoundedPreparedGeneration(generation)
	if ownershipErr != nil {
		return result, fmt.Errorf("own station replay generation: %w", ownershipErr)
	}
	result.Generation = owned
	if generationErr != nil {
		return result, fmt.Errorf("generate station replay: %w", generationErr)
	}
	if err := llm.ValidateExactPreparedGenerationForRequest(prepared, owned); err != nil {
		return result, fmt.Errorf("validate station replay generation: %w", err)
	}
	if owned.ProviderRequestSHA256 != result.PreparedRequestSHA256 || owned.ProviderResponseModel != result.Model {
		return result, fmt.Errorf("station replay generation differs from its prepared authority")
	}
	selection, err := llm.ProviderIdentitySelectionForExpectation(result.ExpectedIdentity)
	if err != nil {
		return result, fmt.Errorf("reconstruct station replay provider policy: %w", err)
	}
	derived, err := llm.DeriveExactProviderIdentityExpectation(
		owned.ProviderIdentityEvidence, selection,
	)
	if err != nil || derived != result.ExpectedIdentity {
		return result, fmt.Errorf("station replay provider identity differs from discovery authority")
	}
	if err := llm.ValidateExactPreparedNaturalUsage(contextTokens, owned.Usage); err != nil {
		return result, fmt.Errorf("validate station replay native usage: %w", err)
	}
	projection, err := assemblyline.NewExactPortableResultProjection(owned.Content)
	if err != nil {
		return result, fmt.Errorf("project station replay response: %w", err)
	}
	if err := (assemblyline.PortableResult{
		JobID: job.ID, Candidate: owned.Content, Projection: &projection,
	}).ValidateFor(job); err != nil {
		return result, fmt.Errorf("validate station replay portable response: %w", err)
	}
	artifact, err := replayExactStationArtifact(job, owned.Content)
	result.Artifact = artifact
	if err != nil {
		return result, &ExactStationReplayArtifactError{Cause: err}
	}
	return result, nil
}
