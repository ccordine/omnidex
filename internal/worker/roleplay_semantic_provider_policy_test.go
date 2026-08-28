package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplaySemanticStationsAndCorrectionsUseSemanticProfile(t *testing.T) {
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
	contextJob, err := assemblyline.NewContextSearchTermCoverageJob(
		assemblyline.ContextSearchTermLeafInput{
			ExactInstruction: "Continue.",
			Scope:            assemblyline.ContextScopeRoleplaySimulation,
			AcceptedTerms:    []string{},
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
				ExactInstruction: "Continue.", RetrievalConcepts: []string{},
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
			SelectedAuthorities: []assemblyline.ContextCandidateAuthority{candidate},
			Scope:               assemblyline.ContextScopeRoleplaySimulation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	baseJobs := []assemblyline.PortableJob{
		canonJob, actionJob, contextJob, relevanceJob, minificationJob,
	}
	for _, base := range baseJobs {
		correction, err := assemblyline.NewRetainedResponseCorrectionJob(
			base, "candidate violates its exact station contract", "invalid",
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, job := range []assemblyline.PortableJob{base, correction} {
			selection, err := providerSelectionForPortableJob(job, "semantic-model:latest", 8192)
			if err != nil {
				t.Fatal(err)
			}
			if selection.ProfilePolicy != llm.ProviderIdentityProfileRoleplaySemanticCompletion {
				t.Fatalf("work %q for %q used provider profile %q",
					job.Kind, base.Kind, selection.ProfilePolicy)
			}
		}
	}
}

func TestAssistantContextAndCorrectionRemainStrict(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewContextSearchTermCoverageJob(
		assemblyline.ContextSearchTermLeafInput{
			ExactInstruction: "Recall it.", AcceptedTerms: []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := assemblyline.NewRetainedResponseCorrectionJob(
		job, "coverage value is not registered", "unsupported",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []assemblyline.PortableJob{job, correction} {
		selection, err := providerSelectionForPortableJob(candidate, "strict-model:latest", 8192)
		if err != nil {
			t.Fatal(err)
		}
		if selection.ProfilePolicy != "" {
			t.Fatalf("assistant work %q escaped strict policy: %q", candidate.Kind, selection.ProfilePolicy)
		}
	}
}
