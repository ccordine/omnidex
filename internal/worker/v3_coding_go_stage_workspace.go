package worker

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/experiment"
	"github.com/gryph/omnidex/internal/queue"
)

type directCodingGoStageWorkspace struct {
	root            string
	cacheRoot       string
	moduleCacheRoot string
	outputRoot      string
	session         *directCodingSession
	profile         directCodingProjectVersionProfile
	experiment      *experiment.Workspace
}

func newDirectCodingGoStageWorkspace(
	session *directCodingSession,
	program directCodingProgram,
) (_ *directCodingGoStageWorkspace, resultErr error) {
	return openDirectCodingGoStageWorkspace(session, program, queue.VerificationIsolatedInstall)
}

func openDirectCodingGoStageWorkspace(session *directCodingSession, program directCodingProgram, phase queue.VerificationCommandPhase) (_ *directCodingGoStageWorkspace, resultErr error) {
	container, err := openDirectCodingExperiment(session, program.Project.Profile)
	if err != nil {
		return nil, err
	}
	workspace := &directCodingGoStageWorkspace{
		root: experiment.WorkingDirectory, cacheRoot: "/tmp/omnidex-go-cache", moduleCacheRoot: "/tmp/omnidex-go-modules",
		outputRoot: "/tmp/omnidex-go-output", session: session, profile: program.Project.Profile, experiment: container,
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, workspace.Close())
		}
	}()
	if err := workspace.verifyToolchain(phase); err != nil {
		return nil, err
	}
	return workspace, nil
}

func (workspace *directCodingGoStageWorkspace) VerifyTask(
	program *directCodingProgram,
	context assemblyline.ApplicationTaskContext,
	testName string,
) error {
	if program == nil {
		return fmt.Errorf("Go task verification requires a compiled program")
	}
	projected, err := projectDirectCodingGoTaskVerificationProgram(*program, context)
	if err != nil {
		return err
	}
	commands := []directCodingVerificationCommand{}
	command, err := directCodingGoVerificationCommand(
		workspace.root, workspace.cacheRoot, workspace.moduleCacheRoot,
		"test", "-count=1", "-run", "^"+testName+"$", "./...",
	)
	if err != nil {
		return err
	}
	commands = append(commands, command)
	return workspace.verify(&projected, queue.VerificationIsolatedTask, commands, false)
}

func projectDirectCodingGoTaskVerificationProgram(
	program directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) (directCodingProgram, error) {
	if err := context.ValidateFor(program.Workload); err != nil {
		return directCodingProgram{}, err
	}
	files, err := program.Coverage.FilesForTask(context.Task.TaskID)
	if err != nil {
		return directCodingProgram{}, err
	}
	projected := program
	projected.TargetTree.Paths = make([]string, 0, len(files))
	for _, file := range files {
		projected.TargetTree.Paths = append(projected.TargetTree.Paths, file.Path)
	}
	return projected, nil
}

func (workspace *directCodingGoStageWorkspace) VerifyFinal(
	program *directCodingProgram,
) error {
	return workspace.verifyFinal(program, queue.VerificationIsolatedFinal)
}

func (workspace *directCodingGoStageWorkspace) verifyFinal(program *directCodingProgram, phase queue.VerificationCommandPhase) error {
	commands := make([]directCodingVerificationCommand, 0, 3)
	for _, arguments := range [][]string{
		{"test", "-count=1", "./..."},
		{"vet", "./..."},
		{"build", "-o", filepath.Join(workspace.outputRoot, "application"), "./..."},
	} {
		command, err := directCodingGoVerificationCommand(
			workspace.root, workspace.cacheRoot, workspace.moduleCacheRoot, arguments...,
		)
		if err != nil {
			return err
		}
		commands = append(commands, command)
	}
	return workspace.verify(program, phase, commands, true)
}

func (workspace *directCodingGoStageWorkspace) VerifyProgress(program *directCodingProgram) error {
	command, err := directCodingGoVerificationCommand(
		workspace.root, workspace.cacheRoot, workspace.moduleCacheRoot,
		"test", "-count=1", "./...",
	)
	if err != nil {
		return err
	}
	return workspace.verify(program, queue.VerificationIsolatedTask, []directCodingVerificationCommand{command}, false)
}

func (workspace *directCodingGoStageWorkspace) verify(
	program *directCodingProgram,
	phase queue.VerificationCommandPhase,
	commands []directCodingVerificationCommand,
	complete bool,
) (resultErr error) {
	if workspace == nil || workspace.session == nil || workspace.root == "" || program == nil {
		return fmt.Errorf("Go stage verification requires one active isolated workspace and program")
	}
	if len(commands) == 0 {
		return fmt.Errorf("Go stage verification requires at least one exact command")
	}
	assembly, err := directCodingAssemblyFromProgram(*program)
	if err != nil {
		return err
	}
	if complete {
		err = validateDirectCodingProgramAssembly(*program, assembly)
	} else {
		err = validateDirectCodingProjectedProgramAssembly(*program, assembly)
	}
	if err != nil {
		return err
	}
	if err := workspace.reset(phase); err != nil {
		return err
	}
	if err := workspace.experiment.Write(workspace.session.runtime.ctx, directCodingExperimentFiles(assembly)); err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, validateDirectCodingExperimentAssembly(workspace.session.runtime.ctx, workspace.experiment, assembly))
	}()
	if err := workspace.verifyFormatting(phase, assembly); err != nil {
		return err
	}
	for _, command := range commands {
		if _, err := workspace.run(phase, command); err != nil {
			return err
		}
	}
	return nil
}

func (workspace *directCodingGoStageWorkspace) verifyToolchain(
	phase queue.VerificationCommandPhase,
) error {
	result, err := workspace.run(phase, directCodingGoVersionCommand())
	if err != nil {
		return fmt.Errorf("observe Go toolchain version: %w", err)
	}
	if strings.TrimSpace(string(result.Stderr)) != "" {
		return fmt.Errorf("Go version probe wrote unexpected stderr")
	}
	return validateDirectCodingGoToolchainVersion(workspace.profile, result.Stdout)
}

func (workspace *directCodingGoStageWorkspace) verifyFormatting(
	phase queue.VerificationCommandPhase,
	assembly directCodingAssembly,
) error {
	paths := directCodingGoAssemblySourcePaths(assembly)
	command, err := directCodingGoFormatCheckCommand(
		workspace.root, workspace.cacheRoot, workspace.moduleCacheRoot, paths...,
	)
	if err != nil {
		return err
	}
	result, err := workspace.run(phase, command)
	if err != nil {
		return err
	}
	return validateDirectCodingGoFormatCheck(result)
}

func directCodingGoAssemblySourcePaths(assembly directCodingAssembly) []string {
	paths := make([]string, 0, len(assembly.Files))
	for _, file := range assembly.Files {
		if filepath.Ext(file.Path) == ".go" {
			paths = append(paths, file.Path)
		}
	}
	sort.Strings(paths)
	return paths
}

func (workspace *directCodingGoStageWorkspace) run(phase queue.VerificationCommandPhase, command directCodingVerificationCommand) (directCodingVerificationCommandResult, error) {
	return workspace.session.runRecordedDockerVerificationCommand(workspace.experiment, phase, command)
}

func (workspace *directCodingGoStageWorkspace) reset(phase queue.VerificationCommandPhase) error {
	_, err := workspace.run(phase, directCodingGoSourceResetCommand())
	return err
}

func directCodingGoSourceResetCommand() directCodingVerificationCommand {
	return directCodingVerificationCommand{
		Argv:    []string{"/bin/sh", "-c", "rm -rf -- /workspace/* /workspace/.[!.]* /workspace/..?* /tmp/omnidex-go-output && mkdir -p /tmp/omnidex-go-output"},
		Timeout: defaultDirectCodingVerificationTimeout,
	}
}

func (workspace *directCodingGoStageWorkspace) Close() error {
	if workspace == nil {
		return nil
	}
	if err := workspace.experiment.Close(); err != nil {
		return err
	}
	workspace.experiment = nil
	workspace.root, workspace.cacheRoot, workspace.moduleCacheRoot, workspace.outputRoot = "", "", "", ""
	return nil
}
