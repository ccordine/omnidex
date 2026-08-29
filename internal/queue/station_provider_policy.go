package queue

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

// PortableJobProviderIdentityProfilePolicy is the single code-owned provider
// classifier shared by durable gap validation and provider selection. It
// inspects typed work and code-owned scope, never user or model wording.
func PortableJobProviderIdentityProfilePolicy(
	job assemblyline.PortableJob,
) (llm.ProviderIdentityProfilePolicy, error) {
	if err := job.Validate(); err != nil {
		return "", err
	}
	switch job.Kind {
	case assemblyline.WorkRoleplayGroundedResponseText:
		return llm.ProviderIdentityProfileRoleplayRawCompletion, nil
	case assemblyline.WorkConversationResponse:
		var input assemblyline.ConversationResponseInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", fmt.Errorf("decode conversation provider policy: %w", err)
		}
		if input.RoleplayIdentity != nil {
			return llm.ProviderIdentityProfileRoleplayRawCompletion, nil
		}
		return "", nil
	case assemblyline.WorkRoleplayCanonFactCoverage,
		assemblyline.WorkRoleplayCanonFact,
		assemblyline.WorkRoleplayGroundedResponseEvidenceRelation,
		assemblyline.WorkRoleplayOngoingAction:
		return llm.ProviderIdentityProfileRoleplaySemanticCompletion, nil
	case assemblyline.WorkContextRelevanceSelection:
		var input assemblyline.ContextRelevanceSelectionInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", fmt.Errorf("decode context-relevance provider policy: %w", err)
		}
		return contextProviderIdentityProfilePolicy(input.Authority.Scope)
	case assemblyline.WorkContextMinification:
		var input assemblyline.ContextMinificationInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return "", fmt.Errorf("decode context-minification provider policy: %w", err)
		}
		return contextProviderIdentityProfilePolicy(input.Scope)
	default:
		return "", nil
	}
}

func contextProviderIdentityProfilePolicy(
	scope assemblyline.ContextScope,
) (llm.ProviderIdentityProfilePolicy, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if scope == assemblyline.ContextScopeRoleplaySimulation {
		return llm.ProviderIdentityProfileRoleplaySemanticCompletion, nil
	}
	return "", nil
}
