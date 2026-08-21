package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Service) exactStationContextTokens(
	ctx context.Context,
	job assemblyline.PortableJob,
	model string,
) (int, error) {
	if ctx == nil || s == nil {
		return 0, fmt.Errorf("exact station context resolution requires context and worker")
	}
	raw, err := portableJobUsesRoleplayRawCompletion(job)
	if err != nil {
		return 0, err
	}
	if !raw {
		if err := llm.ValidateInferenceContextTokens(s.inferenceContextTokens); err != nil {
			return 0, err
		}
		return s.inferenceContextTokens, nil
	}
	resolver, ok := s.stationClient.(llm.RoleplayRawContextResolver)
	if !ok || nilWorkerTransport(resolver) {
		return 0, fmt.Errorf("exact station provider does not implement roleplay raw context resolution")
	}
	contextTokens, err := resolver.ResolveRoleplayRawContext(
		ctx, model, s.inferenceContextTokens,
	)
	if err != nil {
		return 0, err
	}
	if err := llm.ValidateRoleplayRawContextTokens(contextTokens); err != nil {
		return 0, err
	}
	if contextTokens > s.inferenceContextTokens {
		return 0, fmt.Errorf("roleplay raw context resolution exceeded configured authority")
	}
	return contextTokens, nil
}

func providerSelectionForPortableJob(
	job assemblyline.PortableJob,
	model string,
	contextTokens int,
) (llm.ProviderIdentitySelection, error) {
	if err := job.Validate(); err != nil {
		return llm.ProviderIdentitySelection{}, err
	}
	selection := llm.ProviderIdentitySelection{
		Model: model, NativeContextLimit: contextTokens,
	}
	raw, err := portableJobUsesRoleplayRawCompletion(job)
	if err != nil {
		return llm.ProviderIdentitySelection{}, err
	}
	if raw {
		selection.ProfilePolicy = llm.ProviderIdentityProfileRoleplayRawCompletion
	}
	return selection, selection.Validate()
}

func portableJobUsesRoleplayRawCompletion(job assemblyline.PortableJob) (bool, error) {
	return queue.PortableJobUsesRoleplayRawCompletion(job)
}
