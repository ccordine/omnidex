package cognition

import (
	"strings"
	"testing"
)

func TestEnvironmentReceiptRequiresExactlyOneExactResult(t *testing.T) {
	expected := testRevision(1)
	episode := EpisodeRef{ID: expected.EpisodeID}
	action := testRegisteredAction(t)
	current := testRevision(2)
	transition := Transition{
		ActionID: action.ID, Previous: &expected, Current: current,
		Observations: []Observation{}, Effects: []Effect{}, Cost: 1,
	}
	receipt, err := NewEnvironmentTransitionReceipt(episode, action, expected, transition)
	if err != nil || receipt.Transition == nil {
		t.Fatalf("NewEnvironmentTransitionReceipt()=%+v error=%v", receipt, err)
	}
	receipt.Failure = &ActionFailure{}
	if err := receipt.Validate(episode); err == nil {
		t.Fatal("receipt accepted both transition and failure")
	}
}

func TestEnvironmentJournalStateSupportsTaskNeutralNonterminalProgress(t *testing.T) {
	t.Parallel()
	episode := EpisodeRef{ID: "episode-journal-progress"}
	startRevision, err := NewWorldRevision(episode.ID, 1, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	current, err := NewWorldRevision(episode.ID, 2, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := NewScenarioRef("scenario-journal-progress", strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	state := EnvironmentJournalState{
		Episode: episode, Scenario: scenario,
		Start: Transition{
			Current: startRevision, Observations: []Observation{}, Effects: []Effect{},
		},
		Current: current,
	}
	if err := state.Validate(); err == nil {
		t.Fatal("nonterminal progress without its exact current receipt was accepted")
	}
	action := testRegisteredAction(t)
	action.EvidenceRefs[0].Revision = startRevision
	transition := Transition{
		ActionID: action.ID, Previous: &startRevision, Current: current,
		Observations: []Observation{}, Effects: []Effect{}, Cost: 1,
	}
	receipt, err := NewEnvironmentTransitionReceipt(episode, action, startRevision, transition)
	if err != nil {
		t.Fatal(err)
	}
	state.CurrentReceipt = &receipt
	if err := state.Validate(); err != nil {
		t.Fatalf("valid nonterminal progress rejected: %v", err)
	}
	state.Current.Number = 1
	if err := state.Validate(); err == nil {
		t.Fatal("revision-one state accepted a hash different from the exact start")
	}
}

func TestEnvironmentJournalStateSupportsExactTerminalStart(t *testing.T) {
	t.Parallel()
	episode := EpisodeRef{ID: "episode-terminal-start"}
	revision, err := NewWorldRevision(episode.ID, 1, strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := NewScenarioRef("scenario-terminal-start", strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	start := Transition{
		Current: revision, Observations: []Observation{}, Effects: []Effect{},
		Terminal: true, PublicOutcome: "goal already satisfied",
	}
	state := EnvironmentJournalState{
		Episode: episode, Scenario: scenario, Start: start, Current: revision, Terminal: true,
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("terminal start rejected: %v", err)
	}
	state.Terminal = false
	if err := state.Validate(); err == nil {
		t.Fatal("terminal start was represented as an active journal")
	}
	state.Terminal = true
	state.CurrentReceipt = &EnvironmentReceipt{}
	if err := state.Validate(); err == nil {
		t.Fatal("terminal start accepted a fabricated action receipt")
	}
}
