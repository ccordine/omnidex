package worker

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/roleplay"
)

type roleplayContextResolverTestClient struct {
	startupTestLLM
	contextTokens int
	calls         int
	model         string
	requested     int
}

func (client *roleplayContextResolverTestClient) ResolveRoleplayCompletionContext(
	_ context.Context,
	model string,
	requested int,
) (int, error) {
	client.calls++
	client.model = model
	client.requested = requested
	return client.contextTokens, nil
}

func TestExactStationContextNegotiationRunsForRoleplayCompletionProfiles(t *testing.T) {
	client := &roleplayContextResolverTestClient{contextTokens: 8192}
	service := &Service{stationClient: client, inferenceContextTokens: 16384}
	roleplayJob, err := roleplayResponseProviderPolicyJob()
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.exactStationContextTokens(t.Context(), roleplayJob, "tinydolphin:latest")
	if err != nil {
		t.Fatal(err)
	}
	if got != 8192 || client.calls != 1 || client.model != "tinydolphin:latest" || client.requested != 16384 {
		t.Fatalf("context=%d client=%+v", got, client)
	}
	semanticJob, err := assemblyline.NewContextSearchTermCoverageJob(
		assemblyline.ContextSearchTermLeafInput{
			ExactInstruction: "Continue.",
			Scope:            assemblyline.ContextScopeRoleplaySimulation,
			AcceptedTerms:    []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err = service.exactStationContextTokens(t.Context(), semanticJob, "semantic-model:latest")
	if err != nil {
		t.Fatal(err)
	}
	if got != 8192 || client.calls != 2 || client.model != "semantic-model:latest" {
		t.Fatalf("semantic context=%d client=%+v", got, client)
	}

	assistantJob, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Explain rain.",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = service.exactStationContextTokens(t.Context(), assistantJob, "qwen3.5:9b")
	if err != nil {
		t.Fatal(err)
	}
	if got != 16384 || client.calls != 2 {
		t.Fatalf("ordinary station context=%d resolver_calls=%d", got, client.calls)
	}
}

func TestRoleplayRawContextNegotiationRequiresExplicitProviderCapability(t *testing.T) {
	service := &Service{stationClient: startupTestLLM{}, inferenceContextTokens: 8192}
	roleplayJob, err := roleplayResponseProviderPolicyJob()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.exactStationContextTokens(t.Context(), roleplayJob, "tinydolphin:latest"); err == nil {
		t.Fatal("roleplay raw context silently used a provider without context-resolution authority")
	}
}

func TestProviderSelectionUsesRawProfileOnlyForRoleplayProse(t *testing.T) {
	roleplayJob, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindStory, ExactInstruction: "Continue.",
		Context: assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		RoleplayIdentity: &assemblyline.RoleplayResponseIdentity{
			CharacterName: "Mara", Summary: "A harbor watchkeeper.", Voice: "Low and precise.",
		},
		RoleplayUserTurn: &assemblyline.RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionDirection,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := providerSelectionForPortableJob(roleplayJob, "story-model:latest", 8192)
	if err != nil {
		t.Fatal(err)
	}
	if selection.ProfilePolicy != llm.ProviderIdentityProfileRoleplayRawCompletion {
		t.Fatalf("roleplay response used policy %q", selection.ProfilePolicy)
	}

	assistantJob, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Explain rain.",
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err = providerSelectionForPortableJob(assistantJob, "strict-model:latest", 8192)
	if err != nil {
		t.Fatal(err)
	}
	if selection.ProfilePolicy != "" {
		t.Fatalf("ordinary assistant response escaped strict profile policy: %q", selection.ProfilePolicy)
	}

}

func TestRoleplayResponseCorrectionRetainsRawProfilePolicy(t *testing.T) {
	roleplayJob, err := roleplayResponseProviderPolicyJob()
	if err != nil {
		t.Fatal(err)
	}
	correction, err := assemblyline.NewRetainedResponseCorrectionJob(
		roleplayJob,
		"text must be nonempty",
		"invalid",
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := providerSelectionForPortableJob(correction, "story-model:latest", 8192)
	if err != nil {
		t.Fatal(err)
	}
	if selection.ProfilePolicy != llm.ProviderIdentityProfileRoleplayRawCompletion {
		t.Fatalf("roleplay correction used policy %q", selection.ProfilePolicy)
	}
}

func TestRoleplayEvidenceRelationAndCorrectionUseSemanticProfilePolicy(t *testing.T) {
	relation, err := assemblyline.NewRoleplayGroundedResponseEvidenceRelationJob(
		assemblyline.RoleplayGroundedEvidenceRelationInput{
			ExactQuestion: "When was the harbor opened?",
			ParagraphText: "The harbor opened in 1902.",
			Evidence: assemblyline.GroundedEvidenceCapsule{
				ID: "source-1", Text: "The harbor opened in 1902.",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := providerSelectionForPortableJob(
		relation, "semantic-model:latest", 8192,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.ProfilePolicy != llm.ProviderIdentityProfileRoleplaySemanticCompletion {
		t.Fatalf("roleplay evidence relation used policy %q", selection.ProfilePolicy)
	}
	correction, err := assemblyline.NewRetainedResponseCorrectionJob(
		relation, "relation is not one registered value", "invalid",
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err = providerSelectionForPortableJob(
		correction, "semantic-model:latest", 8192,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.ProfilePolicy != llm.ProviderIdentityProfileRoleplaySemanticCompletion {
		t.Fatalf("roleplay evidence relation correction used policy %q", selection.ProfilePolicy)
	}
}

func roleplayResponseProviderPolicyJob() (assemblyline.PortableJob, error) {
	return assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindStory, ExactInstruction: "Continue.",
		Context: assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		RoleplayIdentity: &assemblyline.RoleplayResponseIdentity{
			CharacterName: "Mara", Summary: "A harbor watchkeeper.", Voice: "Low and precise.",
		},
		RoleplayUserTurn: &assemblyline.RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionDirection,
		},
	})
}
