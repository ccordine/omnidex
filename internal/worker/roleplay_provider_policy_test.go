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

func (client *roleplayContextResolverTestClient) ResolveRoleplayRawContext(
	_ context.Context,
	model string,
	requested int,
) (int, error) {
	client.calls++
	client.model = model
	client.requested = requested
	return client.contextTokens, nil
}

func TestExactStationContextNegotiationRunsOnlyForRoleplayRawProse(t *testing.T) {
	client := &roleplayContextResolverTestClient{contextTokens: 4096}
	service := &Service{stationClient: client, inferenceContextTokens: 8192}
	roleplayJob, err := roleplayResponseProviderPolicyJob()
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.exactStationContextTokens(t.Context(), roleplayJob, "tinydolphin:latest")
	if err != nil {
		t.Fatal(err)
	}
	if got != 4096 || client.calls != 1 || client.model != "tinydolphin:latest" || client.requested != 8192 {
		t.Fatalf("context=%d client=%+v", got, client)
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
	if got != 8192 || client.calls != 1 {
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
		`{"schema":"omnidex.conversation-response.v1","text":""}`,
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
