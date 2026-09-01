package worker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

var directCodingTypeScriptGeneratedHostPaths = []string{"node_modules", "dist", ".vite"}

func (s *directCodingSession) verifyAuthoritativeTypeScriptWorkspace(
	program directCodingProgram,
	assembly directCodingAssembly,
) (resultErr error) {
	if err := s.runtime.svc.requireWorkspaceScopeForV3Job(
		s.runtime.claim.Job, s.root,
	); err != nil {
		return fmt.Errorf("validate authoritative host workspace before command verification: %w", err)
	}
	if err := validateDirectCodingProgramAssembly(program, assembly); err != nil {
		return fmt.Errorf("validate authoritative in-memory assembly before host commands: %w", err)
	}
	if err := validateDirectCodingAssemblyAtRoot(s.root, assembly); err != nil {
		return err
	}
	generatedRoots, err := snapshotDirectCodingGeneratedHostPaths(s.root)
	if err != nil {
		return err
	}
	for _, relative := range directCodingTypeScriptGeneratedHostPaths {
		if generatedRoots[relative] {
			return fmt.Errorf(
				"authoritative host verification refuses unowned pre-existing generated path %s",
				relative,
			)
		}
	}
	cacheRoot, err := os.MkdirTemp("", "omnidex-host-npm-cache-")
	if err != nil {
		return fmt.Errorf("create isolated host-verification npm cache: %w", err)
	}
	defer func() {
		cleanupCommand := directCodingTypeScriptHostCleanupCommand(
			directCodingGeneratedHostPathsCreatedByAttempt(generatedRoots),
		)
		_, cleanupErr := s.runRecordedVerificationCommand(
			s.root, queue.VerificationHostCleanup, cleanupCommand, true,
		)
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("clean generated host verification outputs: %w", cleanupErr)
		}
		generatedErr := validateDirectCodingGeneratedHostPathRetention(s.root, generatedRoots)
		if generatedErr != nil {
			generatedErr = fmt.Errorf("verify generated host output cleanup: %w", generatedErr)
		}
		workspaceErr := validateDirectCodingAssemblyAtRoot(s.root, assembly)
		if workspaceErr != nil {
			workspaceErr = fmt.Errorf("revalidate exact authoritative source after host commands: %w", workspaceErr)
		}
		cacheErr := os.RemoveAll(cacheRoot)
		if cacheErr != nil {
			cacheErr = fmt.Errorf("remove isolated host-verification npm cache: %w", cacheErr)
		}
		resultErr = errors.Join(resultErr, cleanupErr, generatedErr, workspaceErr, cacheErr)
	}()
	for _, component := range []string{"node", "npm"} {
		result, err := s.runRecordedVerificationCommand(
			s.root,
			queue.VerificationHostInstall,
			directCodingToolchainVersionCommand(component),
			true,
		)
		if err != nil {
			return fmt.Errorf("observe authoritative %s toolchain version: %w", component, err)
		}
		if strings.TrimSpace(string(result.Stderr)) != "" {
			return fmt.Errorf("authoritative %s version probe wrote unexpected stderr", component)
		}
		if err := validateDirectCodingToolchainVersion(
			program.Project.Profile, component, result.Stdout,
		); err != nil {
			return err
		}
	}
	install, err := directCodingNPMInstallCommand(cacheRoot)
	if err != nil {
		return err
	}
	if _, err := s.runRecordedVerificationCommand(
		s.root, queue.VerificationHostInstall, install, true,
	); err != nil {
		return fmt.Errorf("authoritative TypeScript dependency installation failed: %w", err)
	}
	for _, command := range directCodingFullTypeScriptStageCommands() {
		if _, err := s.runRecordedVerificationCommand(
			s.root, queue.VerificationHostFinal, command, true,
		); err != nil {
			return err
		}
	}
	if err := validateDirectCodingBrowserProductionArtifacts(s.root); err != nil {
		return err
	}
	return nil
}

func directCodingTypeScriptHostCleanupCommand(paths []string) directCodingVerificationCommand {
	argv := []string{
		"node", "--input-type=module", "--eval",
		"import { rmSync } from 'node:fs'; for (const entry of process.argv.slice(1)) rmSync(entry, { recursive: true, force: true });",
	}
	argv = append(argv, paths...)
	return directCodingVerificationCommand{
		Argv:    argv,
		Timeout: 30 * time.Second,
	}
}

func snapshotDirectCodingGeneratedHostPaths(root string) (map[string]bool, error) {
	present := make(map[string]bool, len(directCodingTypeScriptGeneratedHostPaths))
	for _, relative := range directCodingTypeScriptGeneratedHostPaths {
		info, err := os.Lstat(filepath.Join(root, relative))
		switch {
		case os.IsNotExist(err):
			present[relative] = false
			continue
		case err != nil:
			return nil, fmt.Errorf("inspect generated host path %s: %w", relative, err)
		default:
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("generated host path %s is not one exact directory", relative)
			}
			present[relative] = true
		}
	}
	return present, nil
}

func directCodingGeneratedHostPathsCreatedByAttempt(initial map[string]bool) []string {
	created := make([]string, 0, len(directCodingTypeScriptGeneratedHostPaths))
	for _, relative := range directCodingTypeScriptGeneratedHostPaths {
		if !initial[relative] {
			created = append(created, relative)
		}
	}
	return created
}

func validateDirectCodingGeneratedHostPathRetention(
	root string,
	initial map[string]bool,
) error {
	for _, relative := range directCodingTypeScriptGeneratedHostPaths {
		info, err := os.Lstat(filepath.Join(root, relative))
		if initial[relative] {
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("pre-existing generated host directory %s was removed", relative)
			}
			continue
		}
		if !os.IsNotExist(err) {
			if err != nil {
				return fmt.Errorf("inspect generated host cleanup path %s: %w", relative, err)
			}
			return fmt.Errorf("attempt-created generated host path %s remains after cleanup", relative)
		}
	}
	return nil
}

func validateDirectCodingAssemblyAtRoot(
	root string,
	assembly directCodingAssembly,
) error {
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("authoritative workspace root is not one exact directory")
	}
	declared := make(map[string]struct{}, len(assembly.Files))
	for _, file := range assembly.Files {
		declared[file.Path] = struct{}{}
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		info, err := os.Lstat(target)
		if err != nil {
			return fmt.Errorf("inspect authoritative assembly file %s: %w", file.Path, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("authoritative assembly file %s is not one exact regular file", file.Path)
		}
		content, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read authoritative assembly file %s: %w", file.Path, err)
		}
		if !bytes.Equal(content, file.Content) || uint32(info.Mode().Perm()) != file.Mode {
			return fmt.Errorf("authoritative assembly file %s differs from in-memory authority", file.Path)
		}
	}
	for _, required := range assembly.RequiredPaths {
		if _, generated := declared[required]; generated {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(required)))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("required authoritative file %s is absent or non-regular", required)
		}
	}
	for _, deleted := range assembly.DeletePaths {
		_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(deleted)))
		if !os.IsNotExist(err) {
			if err != nil {
				return fmt.Errorf("inspect authoritative deletion %s: %w", deleted, err)
			}
			return fmt.Errorf("authoritative deletion %s is still present", deleted)
		}
	}
	for _, file := range assembly.Files {
		if file.MoveFrom == "" {
			continue
		}
		if _, retained := declared[file.MoveFrom]; retained {
			continue
		}
		_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(file.MoveFrom)))
		if !os.IsNotExist(err) {
			if err != nil {
				return fmt.Errorf("inspect authoritative move source %s: %w", file.MoveFrom, err)
			}
			return fmt.Errorf("authoritative move source %s is still present", file.MoveFrom)
		}
	}
	return nil
}
