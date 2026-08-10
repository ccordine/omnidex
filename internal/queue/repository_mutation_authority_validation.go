package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

func validateRepositoryMutationCommand(command RepositoryMutationCommand) error {
	if err := validateStepAttemptAuthority(command.stepAttemptAuthority()); err != nil {
		return fmt.Errorf("repository mutation authority: %w", err)
	}
	for _, identity := range []struct {
		name, value, prefix string
	}{
		{name: "contract identity", value: command.ContractID, prefix: "change_contract_"},
		{name: "stage identity", value: command.StageID, prefix: "repository_change_stage_"},
		{name: "source snapshot identity", value: command.SourceSnapshotID, prefix: "snapshot_"},
	} {
		if !repositoryMutationOpaqueID(identity.value, identity.prefix) {
			return fmt.Errorf("repository mutation %s is invalid", identity.name)
		}
	}
	if command.Patch == "" || !utf8.ValidString(command.Patch) || strings.ContainsRune(command.Patch, '\x00') {
		return fmt.Errorf("repository mutation patch must be nonempty PostgreSQL-compatible UTF-8")
	}
	if len(command.Patch) > maxRepositoryMutationPatchBytes {
		return fmt.Errorf(
			"repository mutation patch has %d bytes; maximum is %d",
			len(command.Patch), maxRepositoryMutationPatchBytes,
		)
	}
	digest := sha256.Sum256([]byte(command.Patch))
	if command.PatchSHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("repository mutation patch SHA does not match the exact patch")
	}
	if len(command.ChangedFiles) == 0 || len(command.ChangedFiles) > maxRepositoryMutationFiles {
		return fmt.Errorf(
			"repository mutation requires 1-%d exact changed files", maxRepositoryMutationFiles,
		)
	}
	fileIDs := make(map[string]struct{}, len(command.ChangedFiles))
	paths := make(map[string]struct{}, len(command.ChangedFiles))
	previousFileID := ""
	for index, file := range command.ChangedFiles {
		if !repositoryMutationOpaqueID(file.FileID, "file_") {
			return fmt.Errorf("repository mutation changed file %d has an invalid file identity", index)
		}
		if err := validateRepositoryMutationText("changed file identity", file.FileID, 256); err != nil {
			return fmt.Errorf("repository mutation changed file %d: %w", index, err)
		}
		if _, exists := fileIDs[file.FileID]; exists {
			return fmt.Errorf("repository mutation contains duplicate file identity %q", file.FileID)
		}
		if previousFileID != "" && file.FileID < previousFileID {
			return fmt.Errorf("repository mutation changed files must be sorted by file identity")
		}
		previousFileID = file.FileID
		fileIDs[file.FileID] = struct{}{}
		if err := validateRepositoryMutationPath(file.Path); err != nil {
			return fmt.Errorf("repository mutation changed file %d: %w", index, err)
		}
		if _, exists := paths[file.Path]; exists {
			return fmt.Errorf("repository mutation contains duplicate file path %q", file.Path)
		}
		paths[file.Path] = struct{}{}
		if !repositoryMutationHexDigest(file.ExpectedSHA256) {
			return fmt.Errorf("repository mutation changed file %q has invalid post-patch SHA", file.FileID)
		}
		if file.ExpectedSize < 0 {
			return fmt.Errorf("repository mutation changed file %q has invalid post-patch size", file.FileID)
		}
		if !repositoryMutationHexDigest(file.SourceSHA256) {
			return fmt.Errorf("repository mutation changed file %q has invalid source SHA", file.FileID)
		}
		if file.SourceSize < 0 {
			return fmt.Errorf("repository mutation changed file %q has invalid source size", file.FileID)
		}
		if file.SourceSHA256 == file.ExpectedSHA256 && file.SourceSize == file.ExpectedSize {
			return fmt.Errorf("repository mutation changed file %q has identical source and post state", file.FileID)
		}
	}
	return nil
}

func validateRepositoryMutationText(name, value string, maxBytes int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > maxBytes {
		return fmt.Errorf("repository mutation %s must be exact UTF-8 text of 1-%d bytes", name, maxBytes)
	}
	return nil
}

func validateRepositoryMutationPath(value string) error {
	if err := validateRepositoryMutationText("file path", value, 4096); err != nil {
		return err
	}
	if strings.Contains(value, "\\") || strings.HasPrefix(value, "/") ||
		path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("repository mutation file path %q is not an exact workspace-relative slash path", value)
	}
	return nil
}

func repositoryMutationOpaqueID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix) &&
		len(value) == len(prefix)+sha256.Size*2 &&
		repositoryMutationHexDigest(strings.TrimPrefix(value, prefix))
}

func repositoryMutationHexDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
