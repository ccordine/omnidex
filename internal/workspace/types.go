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
	SnapshotSchemaV3       = "omnidex.workspace-path-state.v3"
	maxSnapshotPathBytes   = 4 * 1024
	maxSnapshotTargetBytes = 16 * 1024
)

type EntryKind string

const (
	EntryRegular   EntryKind = "regular"
	EntrySymlink   EntryKind = "symlink"
	EntryDirectory EntryKind = "directory"
)

// Snapshot is the exact state of only the paths named in Paths. It is not an
// inventory of the surrounding workspace, so unrelated files cannot affect a
// reconciliation transaction.
type Snapshot struct {
	Schema      string    `json:"schema"`
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Root        string    `json:"root"`
	GeneratedAt time.Time `json:"generated_at"`
	Paths       []string  `json:"paths"`
	Entries     []Entry   `json:"entries"`
}

type Entry struct {
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	Kind       EntryKind `json:"kind"`
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size"`
	Mode       uint32    `json:"mode"`
	LinkTarget string    `json:"link_target,omitempty"`
}

type snapshotIdentity struct {
	Schema      string               `json:"schema"`
	WorkspaceID string               `json:"workspace_id"`
	Paths       []string             `json:"paths"`
	Entries     []stateEntryIdentity `json:"entries"`
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
	if snapshot.Schema != SnapshotSchemaV3 {
		return fmt.Errorf("workspace path state schema must be %q", SnapshotSchemaV3)
	}
	root, err := exactRootPath(snapshot.Root)
	if err != nil {
		return err
	}
	if snapshot.Root != root || snapshot.WorkspaceID != workspaceID(root) {
		return fmt.Errorf("workspace path state root identity is invalid")
	}
	if snapshot.GeneratedAt.IsZero() || snapshot.GeneratedAt != canonicalTime(snapshot.GeneratedAt) {
		return fmt.Errorf("workspace path state time must use canonical microsecond UTC precision")
	}
	if snapshot.Paths == nil || snapshot.Entries == nil || len(snapshot.Paths) > MaxReconciliationFiles {
		return fmt.Errorf("workspace path state is absent or exceeds %d paths", MaxReconciliationFiles)
	}
	selected := make(map[string]struct{}, len(snapshot.Paths))
	previous := ""
	for _, relative := range snapshot.Paths {
		if err := validateRelativePath(relative); err != nil {
			return err
		}
		if previous != "" && relative <= previous {
			return fmt.Errorf("workspace path state paths must be uniquely sorted")
		}
		previous = relative
		selected[relative] = struct{}{}
	}
	previous = ""
	for _, entry := range snapshot.Entries {
		if err := entry.validate(snapshot.WorkspaceID); err != nil {
			return err
		}
		if _, ok := selected[entry.Path]; !ok {
			return fmt.Errorf("workspace entry %q is outside selected path authority", entry.Path)
		}
		if previous != "" && entry.Path <= previous {
			return fmt.Errorf("workspace path state entries must be uniquely sorted")
		}
		previous = entry.Path
	}
	expected, err := snapshotID(snapshot)
	if err != nil {
		return err
	}
	if snapshot.ID != expected {
		return fmt.Errorf("workspace path state ID differs from its exact content")
	}
	return nil
}

func (entry Entry) validate(workspaceID string) error {
	if err := validateRelativePath(entry.Path); err != nil {
		return fmt.Errorf("workspace entry path: %w", err)
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
		if len(entry.LinkTarget) > maxSnapshotTargetBytes || int64(len(entry.LinkTarget)) != entry.Size ||
			entry.SHA256 != digestBytes([]byte("symlink\x00"+entry.LinkTarget)) {
			return fmt.Errorf("workspace symlink %q has invalid target authority", entry.Path)
		}
	case EntryDirectory:
		if entry.LinkTarget != "" || entry.Size != 0 || entry.SHA256 != digestBytes([]byte("directory\x00")) {
			return fmt.Errorf("workspace directory %q has invalid exact authority", entry.Path)
		}
	default:
		return fmt.Errorf("workspace entry %q has unsupported kind %q", entry.Path, entry.Kind)
	}
	return nil
}

func snapshotID(snapshot Snapshot) (string, error) {
	return stateIDForEntries(snapshot.Schema, snapshot.WorkspaceID, snapshot.Paths, snapshot.Entries)
}

func canonicalStateIdentityJSON(identity snapshotIdentity) ([]byte, error) {
	raw, err := json.Marshal(identity)
	if err != nil {
		return nil, fmt.Errorf("encode workspace path state identity: %w", err)
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

func sortSnapshot(snapshot *Snapshot) {
	sort.Strings(snapshot.Paths)
	sort.Slice(snapshot.Entries, func(left, right int) bool {
		return snapshot.Entries[left].Path < snapshot.Entries[right].Path
	})
}

func canonicalTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }
