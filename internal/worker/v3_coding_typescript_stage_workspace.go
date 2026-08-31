package worker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

type directCodingTypeScriptStageWorkspace struct {
	root             string
	cacheRoot        string
	session          *directCodingSession
	profile          directCodingProjectVersionProfile
	packageAuthority map[string]directCodingFileTask
}

func newDirectCodingTypeScriptStageWorkspace(
	session *directCodingSession,
	program directCodingProgram,
) (_ *directCodingTypeScriptStageWorkspace, resultErr error) {
	if session == nil {
		return nil, fmt.Errorf("isolated TypeScript stage requires one active coding session")
	}
	packageFiles, err := directCodingStagePackageFiles(program.StaticFiles)
	if err != nil {
		return nil, err
	}
	manifest := string(packageFiles[0].Content)
	lock := string(packageFiles[1].Content)
	if err := validatePinnedNPMLockForProfile(manifest, lock, program.Project.Profile); err != nil {
		return nil, fmt.Errorf("validate isolated TypeScript dependency authority: %w", err)
	}
	root, err := os.MkdirTemp("", "omnidex-typescript-stage-")
	if err != nil {
		return nil, fmt.Errorf("create isolated TypeScript stage: %w", err)
	}
	cacheRoot, err := os.MkdirTemp("", "omnidex-npm-cache-")
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, fmt.Errorf("create isolated npm cache: %w", err)
	}
	workspace := &directCodingTypeScriptStageWorkspace{
		root:             root,
		cacheRoot:        cacheRoot,
		session:          session,
		profile:          program.Project.Profile,
		packageAuthority: make(map[string]directCodingFileTask, len(packageFiles)),
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, workspace.Close())
		}
	}()
	for _, packageFile := range packageFiles {
		workspace.packageAuthority[packageFile.Path] = packageFile
		if err := writeDirectCodingStageFile(root, packageFile); err != nil {
			return nil, err
		}
	}
	if err := workspace.verifyToolchain(queue.VerificationIsolatedInstall, true); err != nil {
		return nil, err
	}
	install, err := directCodingNPMInstallCommand(cacheRoot)
	if err != nil {
		return nil, err
	}
	if _, err := session.runRecordedVerificationCommand(
		root, queue.VerificationIsolatedInstall, install, true,
	); err != nil {
		return nil, fmt.Errorf("isolated TypeScript dependency installation failed: %w", err)
	}
	nodeModules, err := os.Lstat(filepath.Join(root, "node_modules"))
	if err != nil || !nodeModules.IsDir() || nodeModules.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("isolated npm installation did not produce one exact node_modules directory")
	}
	return workspace, nil
}

func directCodingStagePackageFiles(
	files []directCodingFileTask,
) ([]directCodingFileTask, error) {
	required := map[string]directCodingFileTask{
		"package.json": {}, "package-lock.json": {},
	}
	counts := map[string]int{}
	for _, file := range files {
		if _, exists := required[file.Path]; exists {
			required[file.Path] = file
			counts[file.Path]++
		}
	}
	ordered := make([]directCodingFileTask, 0, len(required))
	for _, artifactPath := range []string{"package.json", "package-lock.json"} {
		file := required[artifactPath]
		if counts[artifactPath] != 1 || strings.TrimSpace(string(file.Content)) == "" {
			return nil, fmt.Errorf("TypeScript stage requires one non-empty %s", artifactPath)
		}
		ordered = append(ordered, file)
	}
	return ordered, nil
}

func (workspace *directCodingTypeScriptStageWorkspace) Verify(
	program *directCodingProgram,
	phase queue.VerificationCommandPhase,
	commands []directCodingVerificationCommand,
	validators ...func(*directCodingProgram) error,
) error {
	if workspace == nil || workspace.session == nil || workspace.root == "" || program == nil {
		return fmt.Errorf("TypeScript stage verification requires one active isolated workspace and program")
	}
	if len(commands) == 0 {
		return fmt.Errorf("TypeScript stage verification requires at least one exact command")
	}
	for _, validate := range validators {
		if validate == nil {
			return fmt.Errorf("TypeScript stage verification received a nil state validator")
		}
		if err := validate(program); err != nil {
			return fmt.Errorf("validate TypeScript stage authority before materialization: %w", err)
		}
	}
	if err := workspace.resetSource(); err != nil {
		return err
	}
	assembly, err := directCodingAssemblyFromProgram(*program)
	if err != nil {
		return err
	}
	if phase == queue.VerificationIsolatedFinal {
		err = validateDirectCodingProgramAssembly(*program, assembly)
	} else {
		err = validateDirectCodingProjectedProgramAssembly(*program, assembly)
	}
	if err != nil {
		return err
	}
	if err := workspace.writeAssembly(assembly); err != nil {
		return err
	}
	for _, command := range commands {
		if _, err := workspace.session.runRecordedVerificationCommand(
			workspace.root, phase, command, true,
		); err != nil {
			return err
		}
	}
	for _, validate := range validators {
		if err := validate(program); err != nil {
			return fmt.Errorf("revalidate TypeScript stage authority after commands: %w", err)
		}
	}
	if phase == queue.VerificationIsolatedFinal {
		if err := validateDirectCodingBrowserProductionArtifacts(workspace.root); err != nil {
			return err
		}
	}
	return nil
}

func (workspace *directCodingTypeScriptStageWorkspace) verifyToolchain(
	phase queue.VerificationCommandPhase,
	trackWorkspace bool,
) error {
	for _, component := range []string{"node", "npm"} {
		result, err := workspace.session.runRecordedVerificationCommand(
			workspace.root, phase, directCodingToolchainVersionCommand(component), trackWorkspace,
		)
		if err != nil {
			return fmt.Errorf("observe %s toolchain version: %w", component, err)
		}
		if strings.TrimSpace(string(result.Stderr)) != "" {
			return fmt.Errorf("%s version probe wrote unexpected stderr", component)
		}
		if err := validateDirectCodingToolchainVersion(
			workspace.profile, component, result.Stdout,
		); err != nil {
			return err
		}
	}
	return nil
}

func (workspace *directCodingTypeScriptStageWorkspace) resetSource() error {
	entries, err := os.ReadDir(workspace.root)
	if err != nil {
		return fmt.Errorf("read isolated TypeScript stage: %w", err)
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "package.json", "package-lock.json", "node_modules":
			continue
		}
		if err := os.RemoveAll(filepath.Join(workspace.root, entry.Name())); err != nil {
			return fmt.Errorf("reset isolated TypeScript stage path %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (workspace *directCodingTypeScriptStageWorkspace) writeAssembly(
	assembly directCodingAssembly,
) error {
	for _, file := range assembly.Files {
		if authority, packageFile := workspace.packageAuthority[file.Path]; packageFile {
			if string(authority.Content) != string(file.Content) || authority.Mode != file.Mode {
				return fmt.Errorf("staged %s differs from installed dependency authority", file.Path)
			}
			continue
		}
		if err := writeDirectCodingStageFile(workspace.root, file); err != nil {
			return err
		}
	}
	for path, authority := range workspace.packageAuthority {
		content, err := os.ReadFile(filepath.Join(workspace.root, path))
		if err != nil {
			return fmt.Errorf("read staged package authority %s: %w", path, err)
		}
		if string(content) != string(authority.Content) {
			return fmt.Errorf("staged package authority %s changed after installation", path)
		}
	}
	return nil
}

func writeDirectCodingStageFile(root string, file directCodingFileTask) error {
	if _, err := requireExactDirectCodingPath(file.Path); err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(file.Path))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create staged directory for %s: %w", file.Path, err)
	}
	mode := os.FileMode(file.Mode)
	if mode == 0 || mode&^os.FileMode(0o777) != 0 {
		return fmt.Errorf("staged source %s has invalid mode", file.Path)
	}
	if err := os.WriteFile(target, file.Content, mode); err != nil {
		return fmt.Errorf("write staged source %s: %w", file.Path, err)
	}
	return nil
}

func (workspace *directCodingTypeScriptStageWorkspace) Close() error {
	if workspace == nil {
		return nil
	}
	var failures []error
	if workspace.root != "" {
		if err := os.RemoveAll(workspace.root); err != nil {
			failures = append(failures, fmt.Errorf("remove isolated TypeScript stage: %w", err))
		}
		workspace.root = ""
	}
	if workspace.cacheRoot != "" {
		if err := os.RemoveAll(workspace.cacheRoot); err != nil {
			failures = append(failures, fmt.Errorf("remove isolated npm cache: %w", err))
		}
		workspace.cacheRoot = ""
	}
	return errors.Join(failures...)
}
