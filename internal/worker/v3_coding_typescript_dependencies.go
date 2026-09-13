package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/experiment"
	"github.com/gryph/omnidex/internal/queue"
)

func newDirectCodingTypeScriptStageWorkspace(session *directCodingSession, program directCodingProgram) (*directCodingTypeScriptStageWorkspace, error) {
	return openDirectCodingTypeScriptStageWorkspace(session, program, queue.VerificationIsolatedInstall)
}

func openDirectCodingTypeScriptStageWorkspace(session *directCodingSession, program directCodingProgram, phase queue.VerificationCommandPhase) (_ *directCodingTypeScriptStageWorkspace, resultErr error) {
	if session == nil || session.runtime == nil || session.runtime.ctx == nil {
		return nil, fmt.Errorf("TypeScript dependencies require an active coding session")
	}
	packageFiles, err := directCodingStagePackageFiles(program.StaticFiles)
	if err != nil {
		return nil, err
	}
	if err := validatePinnedNPMLockForProfile(string(packageFiles[0].Content), string(packageFiles[1].Content), program.Project.Profile); err != nil {
		return nil, fmt.Errorf("validate TypeScript dependency authority: %w", err)
	}
	reference, err := directCodingExperimentImage(program.Project.Profile)
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
	container, err := docker.OpenAcquisition(ctx, imageID, directCodingExperimentFiles(directCodingAssembly{Files: packageFiles}))
	if err != nil {
		return nil, err
	}
	workspace := &directCodingTypeScriptStageWorkspace{root: experiment.WorkingDirectory, cacheRoot: "/tmp/omnidex-npm-cache", session: session, profile: program.Project.Profile, experiment: container, packageAuthority: make(map[string]directCodingFileTask, len(packageFiles))}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, workspace.Close())
		}
	}()
	for _, file := range packageFiles {
		workspace.packageAuthority[file.Path] = file
	}
	for _, component := range []string{"node", "npm"} {
		result, err := session.runRecordedDockerAcquisitionCommand(container, phase, directCodingToolchainVersionCommand(component))
		if err != nil {
			return nil, fmt.Errorf("observe %s toolchain version: %w", component, err)
		}
		if strings.TrimSpace(string(result.Stderr)) != "" {
			return nil, fmt.Errorf("%s version probe wrote unexpected stderr", component)
		}
		if err := validateDirectCodingToolchainVersion(workspace.profile, component, result.Stdout); err != nil {
			return nil, err
		}
	}
	install, err := directCodingNPMInstallCommand(workspace.cacheRoot)
	if err != nil {
		return nil, err
	}
	if _, err := session.runRecordedDockerAcquisitionCommand(container, phase, install); err != nil {
		return nil, fmt.Errorf("TypeScript dependency installation failed: %w", err)
	}
	if err := container.SealNetwork(ctx); err != nil {
		return nil, err
	}
	if err := validateDirectCodingExperimentAssembly(ctx, container, directCodingAssembly{Files: packageFiles}); err != nil {
		return nil, fmt.Errorf("installed dependency authority changed: %w", err)
	}
	if _, err := workspace.run(phase, directCodingVerificationCommand{Argv: []string{"/bin/sh", "-c", "test -d node_modules && test ! -L node_modules"}, Timeout: defaultDirectCodingVerificationTimeout}); err != nil {
		return nil, fmt.Errorf("npm installation did not produce one exact dependency directory: %w", err)
	}
	return workspace, nil
}

func directCodingStagePackageFiles(files []directCodingFileTask) ([]directCodingFileTask, error) {
	required := map[string]directCodingFileTask{"package.json": {}, "package-lock.json": {}}
	counts := make(map[string]int)
	for _, file := range files {
		if _, exists := required[file.Path]; exists {
			required[file.Path] = file
			counts[file.Path]++
		}
	}
	ordered := make([]directCodingFileTask, 0, len(required))
	for _, name := range []string{"package.json", "package-lock.json"} {
		file := required[name]
		if counts[name] != 1 || strings.TrimSpace(string(file.Content)) == "" {
			return nil, fmt.Errorf("TypeScript stage requires one non-empty %s", name)
		}
		ordered = append(ordered, file)
	}
	return ordered, nil
}
