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
	policy, err := queue.PortableJobProviderIdentityProfilePolicy(job)
	if err != nil {
		return 0, err
	}
	if policy == "" {
		if err := llm.ValidateInferenceContextTokens(s.inferenceContextTokens); err != nil {
			return 0, err
		}
		return s.inferenceContextTokens, nil
	}
	resolver, ok := s.stationClient.(llm.RoleplayCompletionContextResolver)
	if !ok || nilWorkerTransport(resolver) {
		return 0, fmt.Errorf("exact station provider does not implement roleplay completion context resolution")
	}
	contextTokens, err := resolver.ResolveRoleplayCompletionContext(
		ctx, model, s.inferenceContextTokens,
	)
	if err != nil {
		return 0, err
	}
	if err := llm.ValidateRoleplayCompletionContextTokens(contextTokens); err != nil {
		return 0, err
	}
	if contextTokens > s.inferenceContextTokens {
		return 0, fmt.Errorf("roleplay completion context resolution exceeded configured authority")
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
	policy, err := queue.PortableJobProviderIdentityProfilePolicy(job)
	if err != nil {
		return llm.ProviderIdentitySelection{}, err
	}
	selection.ProfilePolicy = policy
	return selection, selection.Validate()
}
