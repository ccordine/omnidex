package worker

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/workspace"
)

func TestWorkspaceDirectoryObservationFollowsCompleteSuccessfulCatalogRead(t *testing.T) {
	for _, fail := range []bool{false, true} {
		access := &observedDirectoryFixture{fail: fail}
		var observations []RuntimeEvent
		runtime := &nativeRuntimeV3{
			workspaceFence: access,
			claim:          &model.ClaimedStep{Authority: model.StepAttemptAuthority{JobID: 1, StepID: 2, Attempt: 3}},
			svc: &Service{runtimeEventSink: func(event RuntimeEvent) error {
				if event.FilePath == "." && access.reads < 2 {
					t.Fatal("catalog observation preceded completed pagination")
				}
				observations = append(observations, event)
				return nil
			}},
		}
		files, err := (&codingAgentEnvironment{runtime: runtime}).ReadFiles(context.Background())
		if fail {
			if err == nil || len(observations) != 0 {
				t.Fatalf("failed directory read was presented as complete: observations=%+v error=%v", observations, err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(files, []string{"a.txt", "src/b.go"}) || len(observations) != 2 {
			t.Fatalf("actual catalog read invisible or changed: files=%v observations=%+v error=%v", files, observations, err)
		}
		for index, file := range []string{".", "src"} {
			event := observations[index]
			if event.Kind != "workspace_directory_read" || event.FileOperation != "list" || event.FilePath != file || event.FileSourcePath != "" || event.JobID != 1 || event.StepID != 2 || event.Attempt != 3 {
				t.Fatalf("directory read changed its actual scope: %+v", event)
			}
		}
	}
}

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

type observedDirectoryFixture struct {
	workspace.Access
	reads int
	fail  bool
}

func (access *observedDirectoryFixture) ReadDirectory(_ context.Context, directory, after string, _ int) (workspace.DirectoryPage, error) {
	access.reads++
	if access.fail && access.reads == 2 {
		return workspace.DirectoryPage{}, errors.New("fenced directory unavailable")
	}
	if directory == "src" {
		return workspace.DirectoryPage{Entries: []workspace.Entry{{Path: "src/b.go", Kind: workspace.EntryFile}}}, nil
	}
	if after == "" {
		return workspace.DirectoryPage{Entries: []workspace.Entry{
			{Path: ".env.example", Kind: workspace.EntryFile},
			{Path: "a.txt", Kind: workspace.EntryFile},
		}, HasMore: true}, nil
	}
	return workspace.DirectoryPage{Entries: []workspace.Entry{{Path: "src", Kind: workspace.EntryDirectory}}}, nil
}
