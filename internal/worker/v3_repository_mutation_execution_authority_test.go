package worker

import (
	"strings"
	"testing"
)

func TestRepositoryMutationRefreshAuthorityRejectsInvalidSnapshotAndAnalysis(t *testing.T) {
	t.Parallel()
	snapshot, _ := existingRepositoryVerificationFixture(t)
	refreshed := existingRepositoryRefreshedIndex(t, snapshot.Root)
	refreshed.Snapshot.ID = "snapshot_" + strings.Repeat("f", 64)
	if err := validateRepositoryRefreshAuthority(refreshed); err == nil ||
		!strings.Contains(err.Error(), "snapshot authority") {
		t.Fatalf("invalid refreshed snapshot error=%v", err)
	}

	refreshed = existingRepositoryRefreshedIndex(t, snapshot.Root)
	refreshed.Analyses[0].Complete = false
	if err := validateRepositoryRefreshAuthority(refreshed); err == nil ||
		!strings.Contains(err.Error(), "analysis authority") {
		t.Fatalf("invalid refreshed analysis error=%v", err)
	}
}
