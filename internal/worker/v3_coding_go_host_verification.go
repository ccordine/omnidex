package worker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *directCodingSession) verifyAuthoritativeGoWorkspace(
	program directCodingProgram,
	assembly directCodingAssembly,
) (resultErr error) {
	if err := s.runtime.svc.requireWorkspaceScopeForV3Job(
		s.runtime.claim.Job, s.root,
	); err != nil {
		return fmt.Errorf("validate authoritative host workspace before Go verification: %w", err)
	}
	if err := validateDirectCodingProgramAssembly(program, assembly); err != nil {
		return fmt.Errorf("validate authoritative in-memory Go assembly before host commands: %w", err)
	}
	if err := validateDirectCodingAssemblyAtRoot(s.root, assembly); err != nil {
		return err
	}
	cacheRoot, err := os.MkdirTemp("", "omnidex-host-go-build-cache-")
	if err != nil {
		return fmt.Errorf("create isolated host Go build cache: %w", err)
	}
	moduleCacheRoot, err := os.MkdirTemp("", "omnidex-host-go-module-cache-")
	if err != nil {
		_ = os.RemoveAll(cacheRoot)
		return fmt.Errorf("create isolated host Go module cache: %w", err)
	}
	outputRoot, err := os.MkdirTemp("", "omnidex-host-go-build-output-")
	if err != nil {
		_ = os.RemoveAll(cacheRoot)
		_ = os.RemoveAll(moduleCacheRoot)
		return fmt.Errorf("create isolated host Go build output: %w", err)
	}
	defer func() {
		workspaceErr := validateDirectCodingAssemblyAtRoot(s.root, assembly)
		if workspaceErr != nil {
			workspaceErr = fmt.Errorf("revalidate exact authoritative Go source after host commands: %w", workspaceErr)
		}
		cacheErr := os.RemoveAll(cacheRoot)
		if cacheErr != nil {
			cacheErr = fmt.Errorf("remove authoritative Go build cache: %w", cacheErr)
		}
		moduleCacheErr := os.RemoveAll(moduleCacheRoot)
		if moduleCacheErr != nil {
			moduleCacheErr = fmt.Errorf("remove authoritative Go module cache: %w", moduleCacheErr)
		}
		outputErr := os.RemoveAll(outputRoot)
		if outputErr != nil {
			outputErr = fmt.Errorf("remove authoritative Go build output: %w", outputErr)
		}
		resultErr = errors.Join(resultErr, workspaceErr, cacheErr, moduleCacheErr, outputErr)
	}()
	version, err := s.runRecordedVerificationCommand(
		s.root, queue.VerificationHostInstall, directCodingGoVersionCommand(), true,
	)
	if err != nil {
		return fmt.Errorf("observe authoritative Go toolchain version: %w", err)
	}
	if strings.TrimSpace(string(version.Stderr)) != "" {
		return fmt.Errorf("authoritative Go version probe wrote unexpected stderr")
	}
	if err := validateDirectCodingGoToolchainVersion(program.Project.Profile, version.Stdout); err != nil {
		return err
	}
	formatCommand, err := directCodingGoFormatCheckCommand(
		s.root, cacheRoot, moduleCacheRoot, directCodingGoAssemblySourcePaths(assembly)...,
	)
	if err != nil {
		return err
	}
	formatResult, err := s.runRecordedVerificationCommand(
		s.root, queue.VerificationHostFinal, formatCommand, true,
	)
	if err != nil {
		return err
	}
	if err := validateDirectCodingGoFormatCheck(formatResult); err != nil {
		return err
	}
	for _, arguments := range [][]string{
		{"test", "-count=1", "./..."},
		{"vet", "./..."},
		{"build", "-o", filepath.Join(outputRoot, "application"), "./..."},
	} {
		command, err := directCodingGoVerificationCommand(
			s.root, cacheRoot, moduleCacheRoot, arguments...,
		)
		if err != nil {
			return err
		}
		if _, err := s.runRecordedVerificationCommand(
			s.root, queue.VerificationHostFinal, command, true,
		); err != nil {
			return err
		}
	}
	return nil
}
