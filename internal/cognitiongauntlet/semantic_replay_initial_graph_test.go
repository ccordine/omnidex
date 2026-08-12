package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticInitialGraphIsExactPublicRootDerivation(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	actor := cognition.AttemptRef{
		JobID: 11, Generation: 12, StepID: 13, Attempt: 14,
		WorkerID: "worker-semantic-initial-graph",
	}
	goal := cognition.GoalExpression{All: []cognition.Predicate{{
		Name: "condition.goal", Args: []string{"target"},
	}}}
	check := cognition.CompletionCheckRef{
		ID: "check.goal", Version: "v1", SHA256: strings.Repeat("b", 64),
	}
	completion, err := cognition.NewCompletionAuthority(
		check, []cognition.PredicateName{"condition.goal"},
	)
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := cognition.DeriveObligationID(
		episode, cognition.InitialObligationGeneration, "", goal, check,
	)
	if err != nil {
		t.Fatal(err)
	}
	root := cognition.ObligationSpec{
		ID: rootID, Desired: goal, DependsOn: []cognition.ObligationID{},
		SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
	}
	graph, err := cognition.NewObligationGraph(
		cognition.InitialObligationGeneration, rootID, []cognition.ObligationSpec{root},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(cognition.InitialObligationGeneration); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(
		rootID, cognition.InitialObligationGeneration, cognition.ObligationActive,
	); err != nil {
		t.Fatal(err)
	}
	snapshot := graph.Snapshot()
	descriptorSHA, err := digestJSON(struct {
		Schema     string              `json:"schema"`
		JobID      int64               `json:"job_id"`
		Generation int64               `json:"generation"`
		StepID     int64               `json:"step_id"`
		EpisodeID  cognition.EpisodeID `json:"episode_id"`
		Graph      string              `json:"graph_sha256"`
	}{
		"omnidex.cognition-obligation-command.v1", actor.JobID, actor.Generation,
		actor.StepID, episode, snapshot.SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	state := &semanticReplayState{
		trace: productionTrace{Header: queue.CognitionSealedTracePage{EpisodeID: episode}},
		goal:  goal, completionAuthority: completion, initialActor: actor,
	}
	recordID := "cognition_graph_command_" + descriptorSHA
	if err := state.verifyInitialObligationGraph(recordID, snapshot); err != nil {
		t.Fatal(err)
	}

	changedActor := *state
	changedActor.initialActor.JobID++
	changedGoal := *state
	changedGoal.goal = state.goal.Clone()
	changedGoal.goal.All[0].Args = []string{"other"}
	extraGoal := cognition.GoalExpression{All: []cognition.Predicate{{Name: "condition.extra"}}}
	extraID, err := cognition.DeriveObligationID(
		episode, cognition.InitialObligationGeneration, rootID, extraGoal, check,
	)
	if err != nil {
		t.Fatal(err)
	}
	extraGraph, err := cognition.NewObligationGraph(
		cognition.InitialObligationGeneration, rootID, []cognition.ObligationSpec{root, {
			ID: extraID, ParentID: rootID, Desired: extraGoal,
			DependsOn: []cognition.ObligationID{}, SupportingRefs: []cognition.EvidenceRef{},
			CompletionCheck: check,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := extraGraph.RefreshReadiness(cognition.InitialObligationGeneration); err != nil {
		t.Fatal(err)
	}
	if err := extraGraph.Transition(
		rootID, cognition.InitialObligationGeneration, cognition.ObligationActive,
	); err != nil {
		t.Fatal(err)
	}
	for name, verify := range map[string]func() error{
		"record_id": func() error {
			return state.verifyInitialObligationGraph(recordID+"-changed", snapshot)
		},
		"actor": func() error {
			return changedActor.verifyInitialObligationGraph(recordID, snapshot)
		},
		"root_goal": func() error {
			return changedGoal.verifyInitialObligationGraph(recordID, snapshot)
		},
		"extra_obligation": func() error {
			return state.verifyInitialObligationGraph(recordID, extraGraph.Snapshot())
		},
	} {
		t.Run(name, func(t *testing.T) {
			if verify() == nil {
				t.Fatal("semantic replay accepted a substituted initial graph")
			}
		})
	}
}
