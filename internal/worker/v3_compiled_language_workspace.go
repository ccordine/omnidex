package worker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

// The stage owns one temporary directory for source and compiler output. It
// records actual stack commands; only owned behavioral assertions establish
// behavioral observations, not compilation alone.
type directCodingCompiledLanguageWorkspace struct {
	root, source, output string
	session              *directCodingSession
	profile              directCodingProjectVersionProfile
}

func newDirectCodingCompiledLanguageWorkspace(session *directCodingSession, program directCodingProgram) (_ *directCodingCompiledLanguageWorkspace, resultErr error) {
	if session == nil {
		return nil, fmt.Errorf("compiled language stage requires one coding session")
	}
	root, err := os.MkdirTemp("", "omnidex-compiled-stage-")
	if err != nil {
		return nil, err
	}
	workspace := &directCodingCompiledLanguageWorkspace{
		root: root, source: filepath.Join(root, "source"), output: filepath.Join(root, "output"),
		session: session, profile: program.Project.Profile,
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, workspace.Close())
		}
	}()
	if err := os.Mkdir(workspace.source, 0o700); err != nil {
		return nil, err
	}
	if err := session.verifyCompiledLanguageToolchain(workspace.source, queue.VerificationIsolatedInstall, workspace.profile); err != nil {
		return nil, err
	}
	return workspace, nil
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

func (workspace *directCodingCompiledLanguageWorkspace) verify(program *directCodingProgram, phase queue.VerificationCommandPhase, complete bool) error {
	if workspace == nil || workspace.root == "" || program == nil || workspace.profile.ID != program.Project.Profile.ID {
		return fmt.Errorf("compiled language verification requires its active stage and selected profile")
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
	if err := os.RemoveAll(workspace.source); err != nil {
		return fmt.Errorf("clear owned compilation stage: %w", err)
	}
	if err := os.Mkdir(workspace.source, 0o700); err != nil {
		return err
	}
	for _, file := range assembly.Files {
		if err := writeDirectCodingStageFile(workspace.source, file); err != nil {
			return err
		}
	}
	return workspace.session.runCompiledLanguageVerification(workspace.source, workspace.output, phase, *program, assembly, complete)
}

func (workspace *directCodingCompiledLanguageWorkspace) Close() error {
	if workspace == nil || workspace.root == "" {
		return nil
	}
	if err := os.RemoveAll(workspace.root); err != nil {
		return fmt.Errorf("remove owned compilation stage and output: %w", err)
	}
	workspace.root, workspace.source, workspace.output = "", "", ""
	return nil
}

func (session *directCodingSession) verifyAuthoritativeCompiledLanguageWorkspace(program directCodingProgram, assembly directCodingAssembly) (resultErr error) {
	if err := session.runtime.svc.requireWorkspaceScopeForV3Job(session.runtime.claim.Job, session.root); err != nil {
		return err
	}
	if err := validateDirectCodingProgramAssembly(program, assembly); err != nil {
		return err
	}
	if err := validateDirectCodingAssemblyAtRoot(session.root, assembly); err != nil {
		return err
	}
	output, err := os.MkdirTemp("", "omnidex-compiled-host-output-")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, os.RemoveAll(output)) }()
	if err := session.verifyCompiledLanguageToolchain(session.root, queue.VerificationHostInstall, program.Project.Profile); err != nil {
		return err
	}
	return session.runCompiledLanguageVerification(session.root, output, queue.VerificationHostFinal, program, assembly, true)
}
