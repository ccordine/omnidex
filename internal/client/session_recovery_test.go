package client

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestFailedStepRequiresLaterMatchingRecovery(t *testing.T) {
	failed := model.Step{ID: 10, JobID: 4, Generation: 2, Action: "v3_coding", SortIndex: 15, Status: model.StepStatusFailed}
	if hasLaterActiveRecoveryStep([]model.Step{failed}, failed) {
		t.Fatal("unrecovered failure became compatible with an active job")
	}
	recovery := failed
	recovery.ID, recovery.Status = 11, model.StepStatusWaiting
	if !hasLaterActiveRecoveryStep([]model.Step{failed, recovery}, failed) {
		t.Fatal("later matching recovery was not recognized")
	}
	recovery.SortIndex++
	if hasLaterActiveRecoveryStep([]model.Step{failed, recovery}, failed) {
		t.Fatal("different ledger position hid the failed step")
	}
}
