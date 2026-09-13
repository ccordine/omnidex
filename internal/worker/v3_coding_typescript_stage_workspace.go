package worker

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/experiment"
	"github.com/gryph/omnidex/internal/queue"
)

type directCodingTypeScriptStageWorkspace struct {
	root, cacheRoot  string
	session          *directCodingSession
	profile          directCodingProjectVersionProfile
	packageAuthority map[string]directCodingFileTask
	experiment       *experiment.Workspace
}

func (workspace *directCodingTypeScriptStageWorkspace) Verify(program *directCodingProgram, phase queue.VerificationCommandPhase, commands []directCodingVerificationCommand, validators ...func(*directCodingProgram) error) (resultErr error) {
	if workspace == nil || workspace.session == nil || workspace.experiment == nil || program == nil {
		return fmt.Errorf("TypeScript verification requires an active Docker workspace and program")
	}
	if len(commands) == 0 {
		return fmt.Errorf("TypeScript verification requires at least one exact command")
	}
	for _, validate := range validators {
		if validate == nil {
			return fmt.Errorf("TypeScript verification received a nil state validator")
		}
		if err := validate(program); err != nil {
			return fmt.Errorf("validate TypeScript authority before materialization: %w", err)
		}
	}
	assembly, err := directCodingAssemblyFromProgram(*program)
	if err != nil {
		return err
	}
	complete := phase == queue.VerificationIsolatedFinal || phase == queue.VerificationHostFinal
	if complete {
		err = validateDirectCodingProgramAssembly(*program, assembly)
	} else {
		err = validateDirectCodingProjectedProgramAssembly(*program, assembly)
	}
	if err != nil {
		return err
	}
	if err := workspace.resetSource(phase); err != nil {
		return err
	}
	if err := workspace.writeAssembly(assembly); err != nil {
		return err
	}
	defer func() {
		if err := validateDirectCodingExperimentAssembly(workspace.session.runtime.ctx, workspace.experiment, assembly); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("revalidate exact isolated TypeScript source after commands: %w", err))
		}
	}()
	for _, command := range commands {
		if _, err := workspace.run(phase, command); err != nil {
			return err
		}
	}
	for _, validate := range validators {
		if err := validate(program); err != nil {
			return fmt.Errorf("revalidate TypeScript authority after commands: %w", err)
		}
	}
	if complete {
		artifacts, err := workspace.experiment.CollectTree(workspace.session.runtime.ctx, "dist")
		if err != nil {
			return err
		}
		if err := validateDirectCodingBrowserProductionArtifacts(assembly, artifacts); err != nil {
			return err
		}
	}
	return nil
}

func (workspace *directCodingTypeScriptStageWorkspace) run(phase queue.VerificationCommandPhase, command directCodingVerificationCommand) (directCodingVerificationCommandResult, error) {
	return workspace.session.runRecordedDockerVerificationCommand(workspace.experiment, phase, command)
}

func (workspace *directCodingTypeScriptStageWorkspace) resetSource(phase queue.VerificationCommandPhase) error {
	_, err := workspace.run(phase, directCodingVerificationCommand{
		Argv:    []string{"node", "--input-type=module", "--eval", `import { readdirSync, rmSync } from 'node:fs'; const retained = new Set(process.argv.slice(1)); for (const entry of readdirSync('.')) { if (!retained.has(entry)) rmSync(entry, { recursive: true, force: true }); }`, "package.json", "package-lock.json", "node_modules"},
		Timeout: defaultDirectCodingVerificationTimeout,
	})
	return err
}

func (workspace *directCodingTypeScriptStageWorkspace) writeAssembly(assembly directCodingAssembly) error {
	retained := directCodingAssembly{}
	for _, file := range workspace.packageAuthority {
		retained.Files = append(retained.Files, file)
	}
	sort.Slice(retained.Files, func(i, j int) bool { return retained.Files[i].Path < retained.Files[j].Path })
	if len(retained.Files) > 0 {
		if err := validateDirectCodingExperimentAssembly(workspace.session.runtime.ctx, workspace.experiment, retained); err != nil {
			return fmt.Errorf("installed dependency authority changed: %w", err)
		}
	}
	input := directCodingAssembly{}
	for _, file := range assembly.Files {
		if authority, exists := workspace.packageAuthority[file.Path]; exists {
			if !bytes.Equal(authority.Content, file.Content) || authority.Mode != file.Mode {
				return fmt.Errorf("staged %s differs from installed dependency authority", file.Path)
			}
			continue
		}
		input.Files = append(input.Files, file)
	}
	return workspace.experiment.Write(workspace.session.runtime.ctx, directCodingExperimentFiles(input))
}

func (workspace *directCodingTypeScriptStageWorkspace) Close() error {
	if workspace == nil {
		return nil
	}
	if err := workspace.experiment.Close(); err != nil {
		return err
	}
	workspace.experiment = nil
	workspace.root, workspace.cacheRoot = "", ""
	return nil
}
