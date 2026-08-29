package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplaySemanticStationsUseSemanticProfile(t *testing.T) {
	t.Parallel()
	source, err := assemblyline.NewRoleplayAssistantCanonSource(
		"Mara", "Mara lowers the bridge.",
	)
	if err != nil {
		t.Fatal(err)
	}
	antecedent := assemblyline.RoleplayCanonAntecedent{
		PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
		ContributionKind:    roleplay.UserContributionDirection,
		ContributionContext: "Continue.",
	}
	canonJob, err := assemblyline.NewRoleplayCanonFactCoverageJob(
		assemblyline.RoleplayCanonFactLeafInput{
			Source: source, AntecedentUserTurn: &antecedent,
			Context: assemblyline.ObjectiveContext{
				Capsules: []assemblyline.ObjectiveContextCapsule{},
			},
			AcceptedFacts: []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	actionJob, err := assemblyline.NewRoleplayOngoingActionJob(
		assemblyline.RoleplayOngoingActionInput{
			CharacterName:     "Mara",
			Source:            assemblyline.RoleplayOngoingActionSourceAssistantResponse,
			ExactContribution: "Mara keeps lowering the bridge.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := assemblyline.NewContextCandidateAuthority(
		"simulation_event", "CTX_8", "The bridge remains raised.",
	)
	if err != nil {
		t.Fatal(err)
	}
	relevanceJob, err := assemblyline.NewContextRelevanceSelectionJob(
		assemblyline.ContextRelevanceSelectionInput{
			Authority: assemblyline.ContextRelevanceInput{
				ExactInstruction:     "Continue.",
				KnownArtifactPaths:   []string{},
				CandidateAuthorities: []assemblyline.ContextCandidateAuthority{candidate},
				MaxSelections:        1, Scope: assemblyline.ContextScopeRoleplaySimulation,
			},
			AcceptedCandidateIDs: []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	minificationJob, err := assemblyline.NewContextMinificationJob(
		assemblyline.ContextMinificationInput{
			ExactInstruction:    "Continue.",
			KnownArtifactPaths:  []string{},
			SelectedAuthorities: []assemblyline.ContextCandidateAuthority{candidate},
			Scope:               assemblyline.ContextScopeRoleplaySimulation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	baseJobs := []assemblyline.PortableJob{
		canonJob, actionJob, relevanceJob, minificationJob,
	}
	for _, job := range baseJobs {
		selection, err := providerSelectionForPortableJob(job, "semantic-model:latest", 8192)
		if err != nil {
			t.Fatal(err)
		}
		if selection.ProfilePolicy != llm.ProviderIdentityProfileRoleplaySemanticCompletion {
			t.Fatalf("work %q used provider profile %q", job.Kind, selection.ProfilePolicy)
		}
	}
}

func TestAssistantContextRemainsStrict(t *testing.T) {
	t.Parallel()
	candidate, err := assemblyline.NewContextCandidateAuthority(
		"conversation_exchange", "CTX_1", "The bridge remains raised.",
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewContextRelevanceSelectionJob(
		assemblyline.ContextRelevanceSelectionInput{
			Authority: assemblyline.ContextRelevanceInput{
				ExactInstruction:     "Recall it.",
				KnownArtifactPaths:   []string{},
				CandidateAuthorities: []assemblyline.ContextCandidateAuthority{candidate},
				MaxSelections:        1,
			},
			AcceptedCandidateIDs: []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := providerSelectionForPortableJob(job, "strict-model:latest", 8192)
	if err != nil {
		t.Fatal(err)
	}
	if selection.ProfilePolicy != "" {
		t.Fatalf("assistant work %q escaped strict policy: %q", job.Kind, selection.ProfilePolicy)
	}
}
