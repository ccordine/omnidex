package worker

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/experiment"
)

func openDirectCodingExperiment(session *directCodingSession, profile directCodingProjectVersionProfile) (*experiment.Workspace, error) {
	if session == nil || session.runtime == nil || session.runtime.ctx == nil {
		return nil, fmt.Errorf("coding experiment requires an active session and context")
	}
	reference, err := directCodingExperimentImage(profile)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(session.runtime.ctx, 10*time.Minute)
	defer cancel()
	docker := experiment.NewDocker()
	imageID, err := docker.ResolveImage(ctx, reference)
	if err != nil {
		return nil, err
	}
	return docker.Open(ctx, imageID, nil)
}

func directCodingExperimentFiles(assembly directCodingAssembly) []experiment.File {
	files := make([]experiment.File, 0, len(assembly.Files))
	for _, file := range assembly.Files {
		files = append(files, experiment.File{Path: file.Path, Content: file.Content, Mode: file.Mode})
	}
	return files
}

func validateDirectCodingExperimentAssembly(parent context.Context, workspace *experiment.Workspace, assembly directCodingAssembly) error {
	paths := make([]string, 0, len(assembly.Files))
	for _, file := range assembly.Files {
		paths = append(paths, file.Path)
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 20*time.Second)
	defer cancel()
	observed, err := workspace.Collect(ctx, paths)
	if err != nil {
		return fmt.Errorf("observe verified Docker source: %w", err)
	}
	if len(observed) != len(assembly.Files) {
		return fmt.Errorf("Docker source observation changed its exact file count")
	}
	for index, file := range assembly.Files {
		if observed[index].Path != file.Path || observed[index].Mode != file.Mode || !bytes.Equal(observed[index].Content, file.Content) {
			return fmt.Errorf("verified Docker source %s changed during command execution", file.Path)
		}
	}
	return nil
}
