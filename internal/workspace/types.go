package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	SnapshotSchemaV1 = "omnidex.workspace-snapshot.v1"

	maxSnapshotEntries     = 100_000
	maxSnapshotPathBytes   = 4 * 1024
	maxSnapshotTargetBytes = 16 * 1024
)

type EntryKind string

const (
	EntryRegular EntryKind = "regular"
	EntrySymlink EntryKind = "symlink"
)

type ExclusionReason string

const (
	ExclusionProtected   ExclusionReason = "protected"
	ExclusionSensitive   ExclusionReason = "sensitive"
	ExclusionAbsent      ExclusionReason = "absent"
	ExclusionUnsupported ExclusionReason = "unsupported"
)

type Snapshot struct {
	Schema      string      `json:"schema"`
	ID          string      `json:"id"`
	WorkspaceID string      `json:"workspace_id"`
	Root        string      `json:"root"`
	GeneratedAt time.Time   `json:"generated_at"`
	Entries     []Entry     `json:"entries"`
	Exclusions  []Exclusion `json:"exclusions"`
	Git         *GitBinding `json:"git,omitempty"`
}

type Entry struct {
	ID               string    `json:"id"`
	Path             string    `json:"path"`
	Kind             EntryKind `json:"kind"`
	SHA256           string    `json:"sha256"`
	Size             int64     `json:"size"`
	Mode             uint32    `json:"mode"`
	LinkTarget       string    `json:"link_target,omitempty"`
	RepositoryFileID string    `json:"repository_file_id,omitempty"`
}

type Exclusion struct {
	Path   string          `json:"path"`
	Reason ExclusionReason `json:"reason"`
}

type GitBinding struct {
	RepositorySnapshotID string `json:"repository_snapshot_id"`
	RepositoryID         string `json:"repository_id"`
	HeadCommit           string `json:"head_commit"`
	StateSHA256          string `json:"state_sha256"`
}

type snapshotIdentity struct {
	Schema      string               `json:"schema"`
	WorkspaceID string               `json:"workspace_id"`
	Entries     []stateEntryIdentity `json:"entries"`
	Exclusions  []Exclusion          `json:"exclusions"`
}

type stateEntryIdentity struct {
	Path       string    `json:"path"`
	Kind       EntryKind `json:"kind"`
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size"`
	Mode       uint32    `json:"mode"`
	LinkTarget string    `json:"link_target,omitempty"`
}

func (snapshot Snapshot) Validate() error {
	if snapshot.Schema != SnapshotSchemaV1 {
		return fmt.Errorf("workspace snapshot schema must be %q", SnapshotSchemaV1)
	}
	root, err := exactRootPath(snapshot.Root)
	if err != nil {
		return err
	}
	if snapshot.Root != root || snapshot.WorkspaceID != workspaceID(root) {
		return fmt.Errorf("workspace snapshot root identity is invalid")
	}
	if snapshot.GeneratedAt.IsZero() || snapshot.GeneratedAt != canonicalTime(snapshot.GeneratedAt) {
		return fmt.Errorf("workspace snapshot generated time must use canonical microsecond UTC precision")
	}
	if snapshot.Entries == nil || snapshot.Exclusions == nil || len(snapshot.Entries) > maxSnapshotEntries {
		return fmt.Errorf("workspace snapshot inventory is absent or exceeds %d entries", maxSnapshotEntries)
	}
	seen := make(map[string]struct{}, len(snapshot.Entries)+len(snapshot.Exclusions))
	previous := ""
	for _, entry := range snapshot.Entries {
		if err := entry.validate(snapshot.WorkspaceID); err != nil {
			return err
		}
		if previous != "" && entry.Path <= previous {
			return fmt.Errorf("workspace snapshot entries must be uniquely sorted")
		}
		previous = entry.Path
		seen[entry.Path] = struct{}{}
	}
	previous = ""
	for _, exclusion := range snapshot.Exclusions {
		if err := validateRelativePath(exclusion.Path); err != nil {
			return fmt.Errorf("workspace exclusion path: %w", err)
		}
		if exclusion.Reason != ExclusionProtected && exclusion.Reason != ExclusionSensitive &&
			exclusion.Reason != ExclusionAbsent && exclusion.Reason != ExclusionUnsupported {
			return fmt.Errorf("workspace exclusion %q has invalid reason %q", exclusion.Path, exclusion.Reason)
		}
		if previous != "" && exclusion.Path <= previous {
			return fmt.Errorf("workspace snapshot exclusions must be uniquely sorted")
		}
		previous = exclusion.Path
		if _, duplicate := seen[exclusion.Path]; duplicate {
			return fmt.Errorf("workspace path %q is both included and excluded", exclusion.Path)
		}
		seen[exclusion.Path] = struct{}{}
	}
	if snapshot.Git != nil {
		if err := snapshot.Git.validate(); err != nil {
			return err
		}
		for _, entry := range snapshot.Entries {
			if !validOpaqueID(entry.RepositoryFileID, "file_") {
				return fmt.Errorf("Git-bound workspace entry %q lacks repository file authority", entry.Path)
			}
		}
	} else {
		for _, entry := range snapshot.Entries {
			if entry.RepositoryFileID != "" {
				return fmt.Errorf("plain workspace entry %q contains Git-only authority", entry.Path)
			}
		}
	}
	expected, err := snapshotID(snapshot)
	if err != nil {
		return err
	}
	if snapshot.ID != expected {
		return fmt.Errorf("workspace snapshot ID differs from its exact content")
	}
	return nil
}

func (entry Entry) validate(workspaceID string) error {
	if err := validateRelativePath(entry.Path); err != nil {
		return fmt.Errorf("workspace entry path: %w", err)
	}
	if protectedPath(entry.Path) {
		return fmt.Errorf("workspace entry %q enters protected authority", entry.Path)
	}
	if entry.ID != opaqueID("workspace_file_", workspaceID, entry.Path) ||
		!validSHA256(entry.SHA256) || entry.Size < 0 || entry.Mode&^uint32(0o777) != 0 {
		return fmt.Errorf("workspace entry %q has invalid exact authority", entry.Path)
	}
	switch entry.Kind {
	case EntryRegular:
		if entry.LinkTarget != "" {
			return fmt.Errorf("regular workspace entry %q cannot have a link target", entry.Path)
		}
	case EntrySymlink:
		if entry.LinkTarget == "" || len(entry.LinkTarget) > maxSnapshotTargetBytes {
			return fmt.Errorf("workspace symlink %q has invalid target authority", entry.Path)
		}
		if err := validateSymlink(entry.Path, entry.LinkTarget); err != nil {
			return err
		}
	default:
		return fmt.Errorf("workspace entry %q has unsupported kind %q", entry.Path, entry.Kind)
	}
	return nil
}

func (binding GitBinding) validate() error {
	if !validOpaqueID(binding.RepositorySnapshotID, "snapshot_") ||
		!validOpaqueID(binding.RepositoryID, "repository_") ||
		!validHexBytes(binding.HeadCommit, 20, 32) || !validSHA256(binding.StateSHA256) {
		return fmt.Errorf("workspace Git binding is incomplete or malformed")
	}
	return nil
}

func snapshotID(snapshot Snapshot) (string, error) {
	return stateIDForEntries(
		snapshot.Schema, snapshot.WorkspaceID, snapshot.Entries, snapshot.Exclusions,
	)
}

func canonicalStateIdentityJSON(identity snapshotIdentity) ([]byte, error) {
	raw, err := json.Marshal(identity)
	if err != nil {
		return nil, fmt.Errorf("encode workspace snapshot identity: %w", err)
	}
	return raw, nil
}

func workspaceID(root string) string { return opaqueID("workspace_", root) }

func opaqueID(prefix string, values ...string) string {
	hash := sha256.New()
	for _, value := range values {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(hash.Sum(nil))
}

func validOpaqueID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil && len(raw) == sha256.Size
}

func validSHA256(value string) bool { return validHexBytes(value, sha256.Size) }

func validHexBytes(value string, sizes ...int) bool {
	raw, err := hex.DecodeString(value)
	if err != nil {
		return false
	}
	for _, size := range sizes {
		if len(raw) == size {
			return true
		}
	}
	return false
}

func validateRelativePath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxSnapshotPathBytes ||
		strings.ContainsAny(value, "\x00\r\n\\") || filepath.IsAbs(value) || path.Clean(value) != value ||
		value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("path %q is not a normalized workspace-relative path", value)
	}
	return nil
}

func protectedPath(value string) bool {
	for _, component := range strings.Split(value, "/") {
		if component == ".git" || component == ".omni" {
			return true
		}
	}
	return false
}

func validateSymlink(relative, target string) error {
	if filepath.IsAbs(target) {
		return fmt.Errorf("workspace symlink %q has an absolute target", relative)
	}
	resolved := filepath.ToSlash(filepath.Clean(filepath.Join(
		filepath.Dir(filepath.FromSlash(relative)), target,
	)))
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") || protectedPath(resolved) {
		return fmt.Errorf("workspace symlink %q escapes included workspace authority", relative)
	}
	return nil
}

func sortSnapshot(snapshot *Snapshot) {
	sort.Slice(snapshot.Entries, func(left, right int) bool {
		return snapshot.Entries[left].Path < snapshot.Entries[right].Path
	})
	sort.Slice(snapshot.Exclusions, func(left, right int) bool {
		return snapshot.Exclusions[left].Path < snapshot.Exclusions[right].Path
	})
}

func canonicalTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }
