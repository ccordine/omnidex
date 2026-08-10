package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestBuildObservationRetentionAcquiresExactOneShotRevisionEvidence(t *testing.T) {
	snapshot, observation := observationRetentionFixture(t)
	mutations, err := BuildObservationRetention(snapshot, "obligation-1", observation)
	if err != nil {
		t.Fatal(err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutations=%d, want 1", len(mutations))
	}
	command, ok := mutations[0].Command().(*workingset.AcquireCommand)
	if !ok {
		t.Fatalf("command=%T, want AcquireCommand", mutations[0].Command())
	}
	wantRef := evidenceLedgerRef(observation.EvidenceRef())
	wantMembership, err := AttentionMembership(
		cognition.AttentionScopeDecision, snapshot.Scope, "obligation-1", observation.Revision.SHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	if command.Request.Ref != wantRef || command.Request.Role != workingset.RoleEvidence ||
		command.Request.Scope != wantMembership.Scope ||
		command.Request.Retention != wantMembership.Retention ||
		command.Request.ByteCost != len(observation.Content) ||
		command.Request.Acquisition.Provider != workingset.ProviderEvidence {
		t.Fatalf("observation acquisition=%+v", command.Request)
	}
	set, err := workingset.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.Apply(command); err != nil {
		t.Fatalf("apply observation retention: %v", err)
	}
}

func TestBuildObservationRetentionReacquiresReleasedExactReference(t *testing.T) {
	snapshot, observation := observationRetentionFixture(t)
	mutations, err := BuildObservationRetention(snapshot, "obligation-1", observation)
	if err != nil {
		t.Fatal(err)
	}
	set, err := workingset.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	acquire := mutations[0].Command().(*workingset.AcquireCommand)
	if _, err := set.Apply(acquire); err != nil {
		t.Fatal(err)
	}
	item := set.Items()[0]
	releaseID, err := workingset.NewCommandID(t.Name(), "release")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.Apply(&workingset.ReleaseCommand{
		CommandID: releaseID, ExpectedVersion: set.Version(), Actor: taskstate.AuthorityCode,
		ItemID: item.ID, Scope: item.Memberships[0].Scope, Reason: "Test releases exact observation evidence.",
	}); err != nil {
		t.Fatal(err)
	}
	mutations, err = BuildObservationRetention(set.Snapshot(), "obligation-2", observation)
	if err != nil {
		t.Fatal(err)
	}
	if len(mutations) != 1 {
		t.Fatalf("reacquire mutations=%d, want 1", len(mutations))
	}
	command, ok := mutations[0].Command().(*workingset.ReacquireCommand)
	if !ok || command.Request.ItemID != item.ID || command.Request.ExpectedReacquisitionCount != 0 {
		t.Fatalf("reacquire command=%+v type=%T", command, mutations[0].Command())
	}
}

func observationRetentionFixture(t *testing.T) (workingset.Snapshot, cognition.Observation) {
	t.Helper()
	ledgerID, err := taskstate.NewLedgerID(taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: 41, RunID: "01234567-89ab-cdef-0123-456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := workingset.Owner{LedgerID: ledgerID, JobID: 41, Generation: 2}
	set, err := workingset.New(owner, workingset.Budget{
		MaxItems: 8, MaxBytes: 4096, MaxPinnedItems: 4, MaxPinnedBytes: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := cognition.NewWorldRevision("episode-1", 2, mappingTextDigest("revision"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewObservation(
		"observation-1", revision, "public_state", "One exact bounded observation.",
	)
	if err != nil {
		t.Fatal(err)
	}
	return set.Snapshot(), observation
}
