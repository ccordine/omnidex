package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
)

type cognitionLifecycleRetirementUnitFixture struct {
	descriptor lifecycleOperationDescriptor
	episode    CognitionEpisode
	graph      CognitionObligationGraphRecord
}

func newCognitionLifecycleRetirementUnitFixture(t *testing.T) cognitionLifecycleRetirementUnitFixture {
	t.Helper()
	operationID, err := NewLifecycleOperationID("unit-lifecycle-retirement")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := lifecycleOperationDescriptor{
		ID: operationID, Kind: LifecycleReplanJob, SHA256: cognitionTestDigest("9"), Payload: []byte(`{}`),
	}
	episodeID := cognition.EpisodeID("episode-lifecycle-unit")
	revision, err := cognition.NewWorldRevision(episodeID, 2, cognitionTestDigest("1"))
	if err != nil {
		t.Fatal(err)
	}
	predicate, err := cognition.NewPredicate("done", []string{"unit"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	check := cognition.CompletionCheckRef{ID: "check.unit", Version: "v1", SHA256: cognitionTestDigest("2")}
	rootID, err := cognition.DeriveObligationID(episodeID, cognition.InitialObligationGeneration, "", goal, check)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := cognition.NewObligationGraph(cognition.InitialObligationGeneration, rootID, []cognition.ObligationSpec{{
		ID: rootID, Desired: goal, DependsOn: []cognition.ObligationID{},
		SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(cognition.InitialObligationGeneration); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(rootID, cognition.InitialObligationGeneration, cognition.ObligationActive); err != nil {
		t.Fatal(err)
	}
	authority := model.StepAttemptAuthority{JobID: 7, Generation: 3, StepID: 11, Attempt: 2, WorkerID: "worker-unit"}
	return cognitionLifecycleRetirementUnitFixture{
		descriptor: descriptor,
		episode:    CognitionEpisode{EpisodeID: episodeID, Authority: authority, CurrentRevision: revision, Status: CognitionEpisodeActive},
		graph:      CognitionObligationGraphRecord{EpisodeID: episodeID, Version: 4, Graph: graph.Snapshot()},
	}
}

func TestCognitionLifecycleRetirementBindsExactOperationEpisodeAndGraph(t *testing.T) {
	fixture := newCognitionLifecycleRetirementUnitFixture(t)
	retirement, err := newCognitionLifecycleRetirement(
		fixture.descriptor, fixture.episode, fixture.graph,
		cognitionruntime.CancellationGenerationRetired,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := retirement.Validate(); err != nil {
		t.Fatal(err)
	}
	changed := retirement
	changed.GraphSHA256 = cognitionTestDigest("f")
	if err := changed.Validate(); err == nil {
		t.Fatal("retirement accepted changed graph projection")
	}
}

func TestCognitionLifecycleSealSetIsCanonicalAndAcceptsExplicitEmpty(t *testing.T) {
	fixture := newCognitionLifecycleRetirementUnitFixture(t)
	empty, err := newCognitionLifecycleSealSet(
		fixture.descriptor, fixture.episode.Authority.JobID,
		fixture.episode.Authority.Generation, []cognitionLifecycleSealEntry{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Entries == nil || len(empty.Entries) != 0 || empty.Validate() != nil {
		t.Fatalf("empty lifecycle seal set=%+v", empty)
	}
	left := cognitionLifecycleSealEntry{
		EpisodeID: "episode-b", RetirementID: "cognition_retirement_" + cognitionTestDigest("a"),
		RetirementSHA256: cognitionTestDigest("a"), TraceSHA256: cognitionTestDigest("b"),
	}
	right := cognitionLifecycleSealEntry{
		EpisodeID: "episode-a", RetirementID: "cognition_retirement_" + cognitionTestDigest("c"),
		RetirementSHA256: cognitionTestDigest("c"), TraceSHA256: cognitionTestDigest("d"),
	}
	set, err := newCognitionLifecycleSealSet(
		fixture.descriptor, fixture.episode.Authority.JobID,
		fixture.episode.Authority.Generation, []cognitionLifecycleSealEntry{left, right},
	)
	if err != nil {
		t.Fatal(err)
	}
	if set.Entries[0].EpisodeID != right.EpisodeID || set.Entries[1].EpisodeID != left.EpisodeID {
		t.Fatalf("seal entries are not canonical: %+v", set.Entries)
	}
	changed := set
	changed.Entries = append([]cognitionLifecycleSealEntry{}, set.Entries...)
	changed.Entries[0].TraceSHA256 = cognitionTestDigest("e")
	if err := changed.Validate(); err == nil {
		t.Fatal("seal set accepted changed trace")
	}
}
