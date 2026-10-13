package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/workspace"
)

func (runtime *nativeRuntimeV3) observeWorkspaceRead(kind, operation, file string) {
	runtime.svc.emitRuntimeEvent(RuntimeEvent{
		JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID,
		Attempt: runtime.claim.Authority.Attempt, Kind: kind, FileOperation: operation, FilePath: file,
	})
}

func (runtime *nativeRuntimeV3) readObservedWorkspaceFile(ctx context.Context, path string) (workspace.File, error) {
	file, err := runtime.workspaceFence.ReadFile(ctx, path)
	if err != nil {
		return workspace.File{}, err
	}
	runtime.observeWorkspaceRead("workspace_file_read", "read", file.Entry.Path)
	return file, nil
}
