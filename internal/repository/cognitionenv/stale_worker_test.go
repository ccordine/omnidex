package cognitionenv

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

type blockingEvidenceBuilder struct {
	pack    repositoryretrieval.EvidencePack
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (builder *blockingEvidenceBuilder) Build(
	_ context.Context,
	_ repositoryretrieval.Request,
) (repositoryretrieval.EvidencePack, error) {
	blocked := false
	builder.once.Do(func() { blocked = true })
	if blocked {
		close(builder.entered)
		<-builder.release
	}
	return builder.pack, nil
}

type fencedMemoryJournal struct {
	*memoryJournal
	mu      sync.Mutex
	current cognition.AttemptRef
}

func (journal *fencedMemoryJournal) authorize(
	_ context.Context,
	actor cognition.AttemptRef,
) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if actor != journal.current {
		return fmt.Errorf("%w: stale test actor", cognition.ErrAuthorityDenied)
	}
	return nil
}

func (journal *fencedMemoryJournal) replace(actor cognition.AttemptRef) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	journal.current = actor
}

func (journal *fencedMemoryJournal) CommitEnvironmentAction(
	ctx context.Context,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
	receipt cognition.EnvironmentReceipt,
) (cognition.EnvironmentReceipt, error) {
	if err := journal.authorize(ctx, receipt.Action.Actor); err != nil {
		return cognition.EnvironmentReceipt{}, err
	}
	return journal.memoryJournal.CommitEnvironmentAction(ctx, episode, scenario, receipt)
}

func TestEnvironmentFencesActorReplacedDuringEvidenceBuild(t *testing.T) {
	investigation, analysis, snapshot := testInvestigation(
		t, repositoryretrieval.OperationSemanticExcerpts,
	)
	episode, err := cognition.NewEpisodeRef("repository-stale-builder-episode")
	if err != nil {
		t.Fatal(err)
	}
	original := cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 2, Attempt: 1, WorkerID: "worker-original",
	}
	replacement := original
	replacement.Attempt, replacement.WorkerID = 2, "worker-replacement"
	journal := &fencedMemoryJournal{
		memoryJournal: &memoryJournal{receipts: make(map[cognition.ActionID]cognition.EnvironmentReceipt)},
		current:       original,
	}
	builder := &blockingEvidenceBuilder{
		pack:    testPack(t, investigation, analysis, snapshot),
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	environment, err := NewEnvironment(
		investigation, episode, builder, journal.authorize, journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	start, err := environment.Start(t.Context(), investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	action := testAction(t, investigation, original, start.Observations[0].EvidenceRef())
	result := make(chan error, 1)
	go func() {
		_, applyErr := environment.Apply(t.Context(), episode, start.Current, action)
		result <- applyErr
	}()
	<-builder.entered
	journal.replace(replacement)
	close(builder.release)
	if err := <-result; !errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("stale post-build commit error=%v, want authority denial", err)
	}
	state, err := journal.EnvironmentState(t.Context(), episode, investigation.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if state.Current != start.Current || state.CurrentReceipt != nil || len(journal.receipts) != 0 {
		t.Fatalf("stale actor mutated journal state=%+v receipts=%d", state, len(journal.receipts))
	}
	reauthorized := action.Clone()
	reauthorized.Actor = replacement
	transition, err := environment.Apply(t.Context(), episode, start.Current, reauthorized)
	if err != nil || !transition.Terminal {
		t.Fatalf("replacement transition=%+v error=%v", transition, err)
	}
}
