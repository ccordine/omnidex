package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestSemanticContextProjectionBindsExactResidentWorkingSetItems(t *testing.T) {
	owner := semanticProjectionOwner(t)
	setA, itemA := semanticProjectionSet(t, owner, "item-a", "a")
	setB, _ := semanticProjectionSet(t, owner, "item-b", "b")
	if setA.ID() != setB.ID() || setA.Version() != setB.Version() {
		t.Fatal("projection forgery fixture lacks equal set identity and version")
	}
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: "semantic-replay", Spec: semanticProjectionSpec(), WorkingSet: setA,
		Materials: []contextbuilder.Material{{
			ItemID: itemA.ID, CurrentRef: itemA.Ref, SourceRefs: []taskstate.Ref{},
			Authority: taskstate.AuthorityCode, Content: "exact resident fact", ByteCost: 19,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := semanticReplayRawRecord(
		"context_projection", 1, 10, 1, projection.ID, semanticReplayJSON(t, projection),
	)
	state := &semanticReplayState{
		workingSet:  setA,
		projections: make(map[cognition.ContextProjectionID]semanticProjectionRecord),
	}
	if _, err := state.mapContextProjection(
		record, semanticReplaySourceForRecord(t, 1, record),
	); err != nil {
		t.Fatalf("exact resident projection rejected: %v", err)
	}
	state = &semanticReplayState{
		workingSet:  setB,
		projections: make(map[cognition.ContextProjectionID]semanticProjectionRecord),
	}
	if _, err := state.mapContextProjection(
		record, semanticReplaySourceForRecord(t, 1, record),
	); err == nil {
		t.Fatal("projection from another equal-version Working Set was accepted")
	}
}

func semanticProjectionOwner(t *testing.T) workingset.Owner {
	t.Helper()
	ledgerID, err := taskstate.NewLedgerID(taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: 1,
		RunID: "01234567-89ab-cdef-0123-456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	return workingset.Owner{LedgerID: ledgerID, JobID: 1, Generation: 1}
}

func semanticProjectionSet(
	t *testing.T,
	owner workingset.Owner,
	id workingset.ItemID,
	hashCharacter string,
) (*workingset.Set, workingset.Item) {
	t.Helper()
	set, err := workingset.New(owner, workingset.Budget{
		MaxItems: 2, MaxBytes: 1024, MaxPinnedItems: 1, MaxPinnedBytes: 512,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := set.Acquire(workingset.AcquireRequest{
		ID: id,
		Ref: taskstate.Ref{
			URI: "task:job/1/entry/" + string(id), Version: "v1",
			Hash: strings.Repeat(hashCharacter, 64), Relation: taskstate.RefSource,
		},
		Role: workingset.RoleFact, Retention: workingset.RetentionJob,
		Scope: set.Scope(), Priority: 10, ByteCost: 64,
		Acquisition: workingset.Acquisition{
			Provider:    workingset.ProviderTaskState,
			OperationID: "acquire-" + string(id), Reason: "semantic replay test",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return set, result.Item
}

func semanticProjectionSpec() contextbuilder.ContextSpec {
	return contextbuilder.ContextSpec{
		Name: "semantic-replay", Version: "v1",
		ScopeRef: taskstate.Ref{
			URI: "task:job/1/node/semantic", Version: "v1",
			Hash: strings.Repeat("e", 64), Relation: taskstate.RefConcerns,
		},
		Required: []contextbuilder.Selector{{
			ID: "fact", Role: workingset.RoleFact, MinItems: 1, MaxItems: 1,
		}},
		AllowedAuthorities:   []taskstate.Authority{taskstate.AuthorityCode},
		MaxItems:             2,
		MaxBytes:             1024,
		MaxAcquisitionRounds: 1,
	}
}
