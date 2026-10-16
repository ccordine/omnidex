package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/workspace"
)

func TestWorkspaceFileObservationFollowsActualReadWithoutSourceContent(t *testing.T) {
	for _, fail := range []bool{false, true} {
		access := &observedFileFixture{fail: fail}
		var observations []RuntimeEvent
		runtime := &nativeRuntimeV3{
			workspaceFence: access,
			claim:          &model.ClaimedStep{Authority: model.StepAttemptAuthority{JobID: 1, StepID: 2, Attempt: 3}},
			svc: &Service{runtimeEventSink: func(event RuntimeEvent) error {
				if access.reads != 1 {
					t.Fatal("file observation preceded its actual fenced read")
				}
				observations = append(observations, event)
				return nil
			}},
		}
		file, err := runtime.readObservedWorkspaceFile(context.Background(), "src/main.go")
		if fail {
			if err == nil || len(observations) != 0 {
				t.Fatalf("failed read was presented as successful: observations=%+v error=%v", observations, err)
			}
			continue
		}
		if err != nil || string(file.Content) != "private source payload" || len(observations) != 1 {
			t.Fatalf("actual file read changed or invisible: file=%+v observations=%+v error=%v", file, observations, err)
		}
		event := observations[0]
		if event.Kind != "workspace_file_read" || event.FileOperation != "read" || event.FilePath != "src/main.go" || event.Detail != "" || event.FileSourcePath != "" || event.AgentActivity != nil || event.AgentCallActivity != nil || event.JobID != 1 || event.StepID != 2 || event.Attempt != 3 {
			t.Fatalf("read observation lost identity or included unrelated contents: %+v", event)
		}
	}
}

type observedFileFixture struct {
	workspace.Access
	reads int
	fail  bool
}

func (access *observedFileFixture) ReadFile(_ context.Context, path string) (workspace.File, error) {
	access.reads++
	if access.fail {
		return workspace.File{}, errors.New("fenced file unavailable")
	}
	return workspace.File{Entry: workspace.Entry{Path: path, Kind: workspace.EntryFile}, Content: []byte("private source payload")}, nil
}
