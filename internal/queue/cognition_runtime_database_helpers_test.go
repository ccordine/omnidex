package queue

import (
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/workingset"
)

type cognitionDatabaseFixture struct {
	Repository *Repository
	Authority  model.StepAttemptAuthority
	EpisodeID  cognition.EpisodeID
	Goal       cognition.GoalExpression
	Catalog    cognition.ActionCatalog
	Check      cognition.CompletionCheckRef
	Evidence   cognition.EvidenceRef
	Start      CognitionEpisodeStart
}

func newCognitionDatabaseFixture(
	t *testing.T,
	repository *Repository,
) cognitionDatabaseFixture {
	t.Helper()
	ctx := t.Context()
	marker := fmt.Sprintf("cognition-runtime-%d", time.Now().UnixNano())
	job := enqueueWorkingSetTestJob(t, ctx, repository, marker)
	authority := claimWorkingSetTestJob(t, ctx, repository, job)
	if _, err := repository.CreateCurrentWorkingSet(ctx, authority, workingset.Budget{
		MaxItems: 32, MaxBytes: 128 * 1024, MaxPinnedItems: 16, MaxPinnedBytes: 96 * 1024,
	}); err != nil {
		t.Fatal(err)
	}
	episodeID := cognition.EpisodeID(fmt.Sprintf("episode-%d", time.Now().UnixNano()))
	revision, err := cognition.NewWorldRevision(episodeID, 1, cognitionTestDigest("a"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewObservation("observation-initial", revision, "public_state", "A bounded public fact is visible.")
	if err != nil {
		t.Fatal(err)
	}
	predicate, err := cognition.NewPredicate("condition", []string{"complete"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := cognition.NewActionSchema(
		"catalog.inspect.v1", "1.0.0", "inspect",
		[]cognition.ActionParameterSpec{{Name: "target", Required: true, MaxBytes: 128}},
		cognition.EvidenceRequired,
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := cognition.NewActionCatalog("catalog.runtime", "1.0.0", []cognition.ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	check := cognition.CompletionCheckRef{
		ID: "check.runtime", Version: "1.0.0", SHA256: cognitionTestDigest("b"),
	}
	completion, err := cognition.NewCompletionAuthority(
		check, []cognition.PredicateName{predicate.Name, "prerequisite"},
	)
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := cognition.DeriveObligationID(episodeID, cognition.InitialObligationGeneration, "", goal, check)
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := cognition.NewScenarioRef("scenario-runtime", cognitionTestDigest("c"))
	if err != nil {
		t.Fatal(err)
	}
	start := CognitionEpisodeStart{
		Authority: authority, EpisodeID: episodeID,
		BrainBootstrap: freshReplayBrainBootstrap(t, cognitionTestBrainBootstrap()),
		Scenario:       scenario, Goal: goal, Completion: completion,
		ActionCatalog: catalog, Budget: cognitionTestRuntimeBudget(),
		Root: cognition.ObligationSpec{
			ID: rootID, Desired: goal, DependsOn: []cognition.ObligationID{},
			SupportingRefs: []cognition.EvidenceRef{observation.EvidenceRef()}, CompletionCheck: check,
		},
		Transition: cognition.Transition{
			Current: revision, Observations: []cognition.Observation{observation},
			Effects: []cognition.Effect{},
		},
	}
	start.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, ctx, episodeID, authority, start.BrainBootstrap.AttestedBrain,
	)
	return cognitionDatabaseFixture{
		Repository: repository,
		Authority:  authority, EpisodeID: episodeID, Goal: goal, Catalog: catalog,
		Check: check, Evidence: observation.EvidenceRef(), Start: start,
	}
}

func cognitionTestRuntimeBudget() cognition.RuntimeBudget {
	return cognition.RuntimeBudget{
		RemainingPolicyCalls: 32, MaxInputBytes: 64 * 1024, MaxInputTokens: 64*1024 + 2,
		MaxOutputBytes: 16 * 1024, MaxOutputTokens: 4 * 1024,
		MaxEvidenceRefs: 16, MaxActionArguments: 8,
		MaxLedgerProposals: 8, MaxAttentionRequests: 8, MaxExpectedEffectBytes: 1024,
	}
}

func cognitionTestFactAuthority() cognitionstate.FactAcceptanceAuthority {
	return cognitionstate.NewNoFactAcceptanceAuthority()
}

func cognitionTestDigest(character string) string {
	result := ""
	for len(result) < 64 {
		result += character
	}
	return result[:64]
}
