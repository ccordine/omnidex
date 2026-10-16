package codex

import (
	"fmt"
	"path/filepath"
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
		if proof.call.Role != codingagent.Execute || len(activity.Paths) != 1 {
			return fmt.Errorf("actual native edit projection changed its grant: role=%s root=%q paths=%q", proof.call.Role, proof.call.Root, activity.Paths)
		}
		path := activity.Paths[0]
		if !filepath.IsAbs(path) {
			path = filepath.Join(proof.call.Root, path)
		}
		if filepath.Clean(path) != filepath.Join(proof.call.Root, "allowed.py") {
			return fmt.Errorf("actual native edit changed its exact target: root=%q paths=%q", proof.call.Root, activity.Paths)
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

func TestNativeEditQualificationBindsTheSameExactTarget(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		name  string
		paths []string
		role  codingagent.Role
		valid bool
	}{
		{"relative", []string{"allowed.py"}, codingagent.Execute, true},
		{"absolute", []string{filepath.Join(root, "allowed.py")}, codingagent.Execute, true},
		{"foreign basename", []string{"other.py"}, codingagent.Execute, false},
		{"foreign root", []string{filepath.Join(root, "..", "allowed.py")}, codingagent.Execute, false},
		{"missing path", nil, codingagent.Execute, false},
		{"multiple paths", []string{"allowed.py", "other.py"}, codingagent.Execute, false},
		{"review edit", []string{"allowed.py"}, codingagent.Review, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := codingagent.Assignment{Root: root, Role: test.role, Task: &codingagent.Task{ID: "greeting"}}
			proof := nativeActivityQualification{call: call}
			activity := agentActivityBase(call, &codingagent.Result{ThreadID: "thread", TurnID: "turn"}, "edit", model.AgentActivityFileEdit, model.AgentActivityCompleted)
			activity.Paths = test.paths
			if err := proof.native(codingagent.NativeEvent{Activity: &activity}); (err == nil) != test.valid {
				t.Fatalf("exact target binding changed: valid=%v error=%v", test.valid, err)
			}
		})
	}
}
