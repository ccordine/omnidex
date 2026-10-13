package codex

import (
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

type nativeActivityQualification struct {
	call                                             codingagent.Assignment
	nativeStarted, nativeCompleted, edits, summaries int
	checksStarted, checksCompleted, checkOutputBytes int
}

func (proof *nativeActivityQualification) native(event codingagent.NativeEvent) error {
	if event.Activity == nil {
		return nil
	}
	activity := *event.Activity
	if err := activity.Validate(); err != nil {
		return err
	}
	if activity.Source != model.AgentActivityNative || activity.Role != string(proof.call.Role) || activity.TaskID != proof.call.Task.ID {
		return fmt.Errorf("actual native activity changed its exact assignment")
	}
	switch activity.Kind {
	case model.AgentActivityCommand:
		if activity.Status == model.AgentActivityStarted {
			proof.nativeStarted++
		} else {
			proof.nativeCompleted++
		}
	case model.AgentActivityFileEdit:
		if proof.call.Role != codingagent.Execute || len(activity.Paths) != 1 || activity.Paths[0] != "allowed.py" {
			return fmt.Errorf("actual native edit projection changed its grant")
		}
		proof.edits++
	case model.AgentActivityReasoningSummary:
		if activity.Text != "" {
			proof.summaries++
		}
	}
	return nil
}

func (proof *nativeActivityQualification) verification(activity model.AgentActivity) error {
	if err := activity.Validate(); err != nil {
		return err
	}
	if activity.Source != model.AgentActivityVerification || activity.Role != string(codingagent.Review) || activity.TaskID != proof.call.Task.ID || activity.CheckID != proof.call.Task.Checks[0].ID {
		return fmt.Errorf("actual native check activity changed its frozen assignment")
	}
	if activity.Kind == model.AgentActivityCommandOutput {
		proof.checkOutputBytes += len(activity.Text)
		return nil
	}
	if activity.Status == model.AgentActivityStarted {
		proof.checksStarted++
	} else {
		proof.checksCompleted++
	}
	return nil
}

func (proof *nativeActivityQualification) verify(t *testing.T, writes int) {
	t.Helper()
	if proof.nativeStarted < 1 || proof.nativeStarted != proof.nativeCompleted || proof.edits != writes || proof.checksStarted != 1 || proof.checksCompleted != 1 || proof.checkOutputBytes == 0 {
		t.Fatalf("actual native/check execution lacks its typed public lifecycle: %+v writes=%d", proof, writes)
	}
	t.Logf("actual activity: native commands=%d confirmed edits=%d public summary chunks=%d; check start/exit=%d/%d output bytes=%d", proof.nativeCompleted, proof.edits, proof.summaries, proof.checksStarted, proof.checksCompleted, proof.checkOutputBytes)
}
