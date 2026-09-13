package experiment

import (
	"context"
	"fmt"
	"time"
)

// Write transfers code-owned files after the caller's explicit source reset.
// It does not conceal mutations by reapplying inputs between commands.
func (workspace *Workspace) Write(ctx context.Context, files []File) error {
	if workspace == nil || ctx == nil {
		return fmt.Errorf("Docker source transfer requires a workspace and context")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	if workspace.acquiring {
		return fmt.Errorf("source transfer requires completed acquisition and a sealed network")
	}
	if err := validateRequest(Request{ImageID: workspace.imageID, Argv: []string{"write"}, Input: files, Timeout: time.Minute}); err != nil {
		return err
	}
	if err := workspace.requireRunning(ctx); err != nil {
		return err
	}
	return workspace.docker.copyInput(ctx, workspace.containerID, files)
}

// CollectTree discovers bounded build outputs within one code-declared directory.
// Every returned file and its directory ancestry pass the same archive validator.
func (workspace *Workspace) CollectTree(parent context.Context, root string) ([]File, error) {
	if workspace == nil || parent == nil {
		return nil, fmt.Errorf("artifact tree collection requires a workspace and context")
	}
	if err := validatePath(root); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	if err := workspace.requireRunning(ctx); err != nil {
		return nil, err
	}
	return workspace.docker.collect(ctx, workspace.containerID, nil, root)
}
