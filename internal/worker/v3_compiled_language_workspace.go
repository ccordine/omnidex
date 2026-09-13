package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/experiment"
	"github.com/gryph/omnidex/internal/queue"
)

type directCodingCompiledLanguageWorkspace struct {
	root, source, output string
	session              *directCodingSession
	profile              directCodingProjectVersionProfile
	experiment           *experiment.Workspace
}

func newDirectCodingCompiledLanguageWorkspace(session *directCodingSession, program directCodingProgram) (*directCodingCompiledLanguageWorkspace, error) {
	return openDirectCodingCompiledLanguageWorkspace(session, program, queue.VerificationIsolatedInstall)
}

func openDirectCodingCompiledLanguageWorkspace(session *directCodingSession, program directCodingProgram, phase queue.VerificationCommandPhase) (_ *directCodingCompiledLanguageWorkspace, resultErr error) {
	if session == nil || session.runtime == nil || session.runtime.ctx == nil {
		return nil, fmt.Errorf("compiled language stage requires one active coding session")
	}
	container, err := openDirectCodingExperiment(session, program.Project.Profile)
	if err != nil {
		return nil, err
	}
	workspace := &directCodingCompiledLanguageWorkspace{
		root: experiment.WorkingDirectory, source: experiment.WorkingDirectory, output: "/tmp/omnidex-compiler-output",
		session: session, profile: program.Project.Profile, experiment: container,
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, workspace.Close())
		}
	}()
	if err := verifyDirectCodingCompiledLanguageToolchain(workspace.run, phase, workspace.profile); err != nil {
		return nil, err
	}
	return workspace, nil
}

func (workspace *directCodingCompiledLanguageWorkspace) run(phase queue.VerificationCommandPhase, command directCodingVerificationCommand) (directCodingVerificationCommandResult, error) {
	return workspace.session.runRecordedDockerVerificationCommand(workspace.experiment, phase, command)
}

func (workspace *directCodingCompiledLanguageWorkspace) VerifyTask(context assemblyline.ApplicationTaskContext, program *directCodingProgram) error {
	if program == nil {
		return fmt.Errorf("task compilation requires a projected program")
	}
	if err := context.ValidateFor(program.Workload); err != nil {
		return err
	}
	files, err := program.Coverage.FilesForTask(context.Task.TaskID)
	if err != nil {
		return err
	}
	projected := *program
	projected.TargetTree.Paths = make([]string, 0, len(files))
	for _, file := range files {
		projected.TargetTree.Paths = append(projected.TargetTree.Paths, file.Path)
	}
	return workspace.verify(&projected, queue.VerificationIsolatedTask, false)
}

func (workspace *directCodingCompiledLanguageWorkspace) VerifyFinal(program *directCodingProgram) error {
	return workspace.verify(program, queue.VerificationIsolatedFinal, true)
}

func (workspace *directCodingCompiledLanguageWorkspace) verify(program *directCodingProgram, phase queue.VerificationCommandPhase, complete bool) (resultErr error) {
	if workspace == nil || workspace.experiment == nil || program == nil || workspace.profile.ID != program.Project.Profile.ID {
		return fmt.Errorf("compiled language verification requires its active Docker stage and selected profile")
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
	commands, err := directCodingCompiledLanguageCommands(workspace.profile, assembly, workspace.source, workspace.output, complete)
	if err != nil {
		return err
	}
	reset := directCodingVerificationCommand{
		Argv:    []string{"/bin/sh", "-c", "rm -rf -- /workspace/* /workspace/.[!.]* /workspace/..?* /tmp/omnidex-compiler-output && mkdir -p /tmp/omnidex-compiler-output/classes"},
		Timeout: defaultDirectCodingVerificationTimeout,
	}
	if _, err := workspace.run(phase, reset); err != nil {
		return err
	}
	if err := workspace.experiment.Write(workspace.session.runtime.ctx, directCodingExperimentFiles(assembly)); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, workspace.validateSource(assembly)) }()
	for _, command := range commands {
		if _, err := workspace.run(phase, command); err != nil {
			return err
		}
	}
	return nil
}

func (workspace *directCodingCompiledLanguageWorkspace) validateSource(assembly directCodingAssembly) error {
	return validateDirectCodingExperimentAssembly(workspace.session.runtime.ctx, workspace.experiment, assembly)
}

func (workspace *directCodingCompiledLanguageWorkspace) Close() error {
	if workspace == nil {
		return nil
	}
	if err := workspace.experiment.Close(); err != nil {
		return err
	}
	workspace.experiment = nil
	workspace.root, workspace.source, workspace.output = "", "", ""
	return nil
}

func (session *directCodingSession) verifyAuthoritativeCompiledLanguageWorkspace(program directCodingProgram, assembly directCodingAssembly) (resultErr error) {
	if err := session.runtime.svc.requireWorkspaceScopeForV3Job(session.runtime.ctx, session.runtime.claim.Job, session.root); err != nil {
		return err
	}
	if err := validateDirectCodingProgramAssembly(program, assembly); err != nil {
		return err
	}
	if err := validateDirectCodingAssembly(session.runtime.ctx, session.runtime.workspaceFence, assembly); err != nil {
		return err
	}
	workspace, err := openDirectCodingCompiledLanguageWorkspace(session, program, queue.VerificationHostInstall)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, workspace.Close(), validateDirectCodingAssembly(session.runtime.ctx, session.runtime.workspaceFence, assembly))
	}()
	return workspace.verify(&program, queue.VerificationHostFinal, true)
}
