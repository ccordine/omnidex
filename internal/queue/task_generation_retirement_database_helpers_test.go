package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5/pgxpool"
)

type taskGenerationRetirementFixture struct {
	Repository *Repository
	Pool       *pgxpool.Pool
	Context    context.Context
	Job        model.Job
	Authority  model.StepAttemptAuthority
	EpisodeID  cognition.EpisodeID
	NodeID     taskstate.NodeID
	WorkingSet workingset.SetID
	Start      CognitionEpisodeStart
}

func startTaskGenerationRetirementFixture(t *testing.T, label string) taskGenerationRetirementFixture {
	t.Helper()
	repository, pool, ctx := replanTestRepository(t)
	return startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx, label)
}

func startTaskGenerationRetirementFixtureIn(
	t *testing.T,
	repository *Repository,
	pool *pgxpool.Pool,
	ctx context.Context,
	label string,
) taskGenerationRetirementFixture {
	t.Helper()
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, label)
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func prepareTaskGenerationRetirementFixture(
	t *testing.T,
	repository *Repository,
	pool *pgxpool.Pool,
	ctx context.Context,
	label string,
) taskGenerationRetirementFixture {
	t.Helper()
	marker := fmt.Sprintf("generation-retirement-%s-%d", label, time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineAssistant, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "worker-"+marker)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claimed step=%+v want job %d", claim, job.ID)
	}
	for claim.Step.Action != replanPlanningBoundary {
		if err := repository.CompleteStep(ctx, CompleteStepCommand{
			OperationID: testLifecycleOperationID(t, "cognition-fixture-prefix", claim.Step.ID),
			Authority:   claim.Authority, StepID: claim.Step.ID,
			Output: "completed cognition fixture prefix step",
		}); err != nil {
			t.Fatal(err)
		}
		claim, err = repository.ClaimNextStep(ctx, "worker-"+marker)
		if err != nil {
			t.Fatal(err)
		}
		if claim == nil || claim.Job.ID != job.ID {
			t.Fatalf("planning claim=%+v want job %d", claim, job.ID)
		}
	}
	budget := workingset.Budget{
		MaxItems: 32, MaxBytes: 128 * 1024,
		MaxPinnedItems: 16, MaxPinnedBytes: 96 * 1024,
	}
	workingSet, err := repository.CreateCurrentWorkingSet(ctx, claim.Authority, budget)
	if err != nil {
		t.Fatal(err)
	}

	episodeID := cognition.EpisodeID("episode_" + marker)
	scenario, err := cognition.NewScenarioRef(
		cognition.ScenarioID("scenario_"+marker), strings.Repeat("a", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	predicate, err := cognition.NewPredicate("target_ready", []string{"target"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := cognition.NewActionSchema(
		"schema_observe", "v1", "observe", nil, cognition.EvidenceOptional,
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := cognition.NewActionCatalog("catalog_retirement", "v1", []cognition.ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	check := cognition.CompletionCheckRef{
		ID: "check_target_ready", Version: "v1", SHA256: strings.Repeat("b", 64),
	}
	completion, err := cognition.NewCompletionAuthority(
		check, []cognition.PredicateName{predicate.Name},
	)
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := cognition.DeriveObligationID(episodeID, 1, "", goal, check)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := cognition.NewWorldRevision(episodeID, 1, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	start := CognitionEpisodeStart{
		Authority: claim.Authority, EpisodeID: episodeID, AttestedBrain: cognitionTestBrain(), Scenario: scenario,
		Goal: goal, Completion: completion, ActionCatalog: catalog, Budget: cognitionTestRuntimeBudget(),
		Root: cognition.ObligationSpec{
			ID: nodeID, Desired: goal, DependsOn: []cognition.ObligationID{},
			SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
		},
		Transition: cognition.Transition{
			Current: revision, Observations: []cognition.Observation{}, Effects: []cognition.Effect{},
		},
	}
	return taskGenerationRetirementFixture{
		Repository: repository, Pool: pool, Context: ctx, Job: job,
		Authority: claim.Authority, EpisodeID: episodeID, NodeID: taskstate.NodeID(nodeID),
		WorkingSet: workingSet.ID, Start: start,
	}
}
