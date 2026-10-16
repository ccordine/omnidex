package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestActiveChannelRecoveryRequiresLaterMatchingStep(t *testing.T) {
	failed := model.Step{ID: 20, JobID: 9, Generation: 1, Action: "v3_coding", SortIndex: 15, Status: model.StepStatusFailed}
	if hasLaterActiveRecoveryStep([]model.Step{failed}, failed) {
		t.Fatal("unrecovered failure became valid channel state")
	}
	recovery := failed
	recovery.ID, recovery.Status = 21, model.StepStatusRunning
	if !hasLaterActiveRecoveryStep([]model.Step{failed, recovery}, failed) {
		t.Fatal("later matching recovery was not recognized")
	}
	recovery.Action = "v3_coding_plan"
	if hasLaterActiveRecoveryStep([]model.Step{failed, recovery}, failed) {
		t.Fatal("different action hid the failed step")
	}
}

func TestRecoveredActiveTurnAllowsLaterTerminalTaskHistory(t *testing.T) {
	job := model.Job{ID: 19, Status: model.JobStatusRunning, Instruction: "Continue the frozen ledger."}
	messages := []model.ChannelMessage{
		{ID: 24, Role: model.ChannelMessageRoleUser, Content: job.Instruction, Turn: &model.ChannelMessageTurnState{JobID: job.ID, Status: job.Status}},
		{ID: 25, Role: model.ChannelMessageRoleUser, Content: "Accidental later task.", Turn: &model.ChannelMessageTurnState{JobID: 20, Status: model.JobStatusCanceled}},
	}
	if err := requireTranscriptActiveTurn(messages, &job, 24); err != nil {
		t.Fatalf("terminal task history displaced recovered active authority: %v", err)
	}
	messages = append(messages, model.ChannelMessage{ID: 26, Role: model.ChannelMessageRoleUser, Content: "Unbound later task."})
	if err := requireTranscriptActiveTurn(messages, &job, 24); err == nil {
		t.Fatal("unbound later user task bypassed recovered active authority")
	}
}
