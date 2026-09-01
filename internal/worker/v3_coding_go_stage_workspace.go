package worker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

type directCodingGoStageWorkspace struct {
	root            string
	cacheRoot       string
	moduleCacheRoot string
	outputRoot      string
	session         *directCodingSession
	profile         directCodingProjectVersionProfile
}

func newDirectCodingGoStageWorkspace(
	session *directCodingSession,
	program directCodingProgram,
) (_ *directCodingGoStageWorkspace, resultErr error) {
	if session == nil {
		return nil, fmt.Errorf("isolated Go stage requires one active coding session")
	}
	root, err := os.MkdirTemp("", "omnidex-go-stage-")
	if err != nil {
		return nil, fmt.Errorf("create isolated Go stage: %w", err)
	}
	cacheRoot, err := os.MkdirTemp("", "omnidex-go-build-cache-")
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, fmt.Errorf("create isolated Go build cache: %w", err)
	}
	moduleCacheRoot, err := os.MkdirTemp("", "omnidex-go-module-cache-")
	if err != nil {
		_ = os.RemoveAll(root)
		_ = os.RemoveAll(cacheRoot)
		return nil, fmt.Errorf("create isolated Go module cache: %w", err)
	}
	outputRoot, err := os.MkdirTemp("", "omnidex-go-build-output-")
	if err != nil {
		_ = os.RemoveAll(root)
		_ = os.RemoveAll(cacheRoot)
		_ = os.RemoveAll(moduleCacheRoot)
		return nil, fmt.Errorf("create isolated Go build output: %w", err)
	}
	workspace := &directCodingGoStageWorkspace{
		root: root, cacheRoot: cacheRoot, moduleCacheRoot: moduleCacheRoot,
		outputRoot: outputRoot, session: session, profile: program.Project.Profile,
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, workspace.Close())
		}
	}()
	if err := workspace.verifyToolchain(queue.VerificationIsolatedInstall, true); err != nil {
		return nil, err
	}
	return workspace, nil
}

func (workspace *directCodingGoStageWorkspace) VerifyTask(
	program *directCodingProgram,
	context assemblyline.ApplicationTaskContext,
	testName string,
) error {
	if program == nil || context.WorkloadSHA256 != program.Workload.SHA256 {
		return fmt.Errorf("Go task verification requires matching program and workload authority")
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
	if context.WorkloadSHA256 == "" || context.WorkloadSHA256 != program.Workload.SHA256 {
		return directCodingProgram{}, fmt.Errorf("Go task projection requires matching workload authority")
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
	return workspace.verify(program, queue.VerificationIsolatedFinal, commands, true)
}

func (workspace *directCodingGoStageWorkspace) verify(
	program *directCodingProgram,
	phase queue.VerificationCommandPhase,
	commands []directCodingVerificationCommand,
	complete bool,
) error {
	if workspace == nil || workspace.session == nil || workspace.root == "" || program == nil {
		return fmt.Errorf("Go stage verification requires one active isolated workspace and program")
	}
	if len(commands) == 0 {
		return fmt.Errorf("Go stage verification requires at least one exact command")
	}
	if err := workspace.reset(); err != nil {
		return err
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
	for _, file := range assembly.Files {
		if err := writeDirectCodingStageFile(workspace.root, file); err != nil {
			return err
		}
	}
	if err := workspace.verifyFormatting(phase, assembly); err != nil {
		return err
	}
	for _, command := range commands {
		if _, err := workspace.session.runRecordedVerificationCommand(
			workspace.root, phase, command, true,
		); err != nil {
			return err
		}
	}
	if err := validateDirectCodingAssemblyAtRoot(workspace.root, assembly); err != nil {
		return fmt.Errorf("revalidate exact isolated Go source after commands: %w", err)
	}
	return nil
}

func (workspace *directCodingGoStageWorkspace) verifyToolchain(
	phase queue.VerificationCommandPhase,
	trackWorkspace bool,
) error {
	result, err := workspace.session.runRecordedVerificationCommand(
		workspace.root, phase, directCodingGoVersionCommand(), trackWorkspace,
	)
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
	result, err := workspace.session.runRecordedVerificationCommand(
		workspace.root, phase, command, true,
	)
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

func (workspace *directCodingGoStageWorkspace) reset() error {
	entries, err := os.ReadDir(workspace.root)
	if err != nil {
		return fmt.Errorf("read isolated Go stage: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(workspace.root, entry.Name())); err != nil {
			return fmt.Errorf("reset isolated Go stage path %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (workspace *directCodingGoStageWorkspace) Close() error {
	if workspace == nil {
		return nil
	}
	var failures []error
	for label, target := range map[string]*string{
		"stage": &workspace.root, "build cache": &workspace.cacheRoot,
		"module cache": &workspace.moduleCacheRoot,
		"build output": &workspace.outputRoot,
	} {
		if *target == "" {
			continue
		}
		if err := os.RemoveAll(*target); err != nil {
			failures = append(failures, fmt.Errorf("remove isolated Go %s: %w", label, err))
		}
		*target = ""
	}
	return errors.Join(failures...)
}
