package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const SnapshotSchemaV1 = "omnidex.repository-snapshot.v1"

type EntryKind string

const (
	EntryRegular EntryKind = "regular"
	EntrySymlink EntryKind = "symlink"
)

type ExclusionReason string

const (
	ExclusionSensitive   ExclusionReason = "sensitive"
	ExclusionAbsent      ExclusionReason = "absent_from_worktree"
	ExclusionUnsupported ExclusionReason = "unsupported_entry"
)

type Snapshot struct {
	Schema         string      `json:"schema"`
	ID             string      `json:"id"`
	RepositoryID   string      `json:"repository_id"`
	Root           string      `json:"root"`
	HeadCommit     string      `json:"head_commit"`
	GitStateSHA256 string      `json:"git_state_sha256"`
	Dirty          bool        `json:"dirty"`
	GeneratedAt    time.Time   `json:"generated_at"`
	Files          []File      `json:"files"`
	Exclusions     []Exclusion `json:"exclusions"`
}

type File struct {
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	Kind       EntryKind `json:"kind"`
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size"`
	Mode       uint32    `json:"mode"`
	Language   string    `json:"language,omitempty"`
	Manifest   string    `json:"manifest,omitempty"`
	Test       bool      `json:"test,omitempty"`
	Generated  bool      `json:"generated,omitempty"`
	LinkTarget string    `json:"link_target,omitempty"`
}

type Exclusion struct {
	Path   string          `json:"path"`
	Reason ExclusionReason `json:"reason"`
}

type snapshotIdentity struct {
	Schema         string      `json:"schema"`
	RepositoryID   string      `json:"repository_id"`
	HeadCommit     string      `json:"head_commit"`
	GitStateSHA256 string      `json:"git_state_sha256"`
	Files          []File      `json:"files"`
	Exclusions     []Exclusion `json:"exclusions"`
}

func (snapshot Snapshot) Validate() error {
	if snapshot.Schema != SnapshotSchemaV1 {
		return fmt.Errorf("repository snapshot schema must be %q", SnapshotSchemaV1)
	}
	if !validOpaqueID(snapshot.RepositoryID, "repository_") {
		return fmt.Errorf("repository snapshot has an invalid repository ID")
	}
	if strings.TrimSpace(snapshot.Root) == "" || !filepath.IsAbs(snapshot.Root) {
		return fmt.Errorf("repository snapshot root must be absolute")
	}
	if !validGitOID(snapshot.HeadCommit) {
		return fmt.Errorf("repository snapshot requires one exact Git HEAD commit")
	}
	if !validSHA256(snapshot.GitStateSHA256) {
		return fmt.Errorf("repository snapshot requires one Git state hash")
	}
	if err := validateCanonicalGeneratedAt("repository snapshot", snapshot.GeneratedAt); err != nil {
		return err
	}
	if snapshot.Files == nil || snapshot.Exclusions == nil {
		return fmt.Errorf("repository snapshot files and exclusions require canonical non-nil collections")
	}
	if err := validateSnapshotFacts(snapshot); err != nil {
		return err
	}
	expected, err := snapshotID(snapshot)
	if err != nil {
		return err
	}
	if snapshot.ID != expected {
		return fmt.Errorf("repository snapshot ID does not match its exact content")
	}
	return nil
}

func canonicalRepositoryTimestamp(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func validateCanonicalGeneratedAt(subject string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s generated time is required", subject)
	}
	if value != canonicalRepositoryTimestamp(value) {
		return fmt.Errorf("%s generated time must use canonical microsecond UTC precision", subject)
	}
	return nil
}

func validateSnapshotFacts(snapshot Snapshot) error {
	seen := make(map[string]struct{}, len(snapshot.Files)+len(snapshot.Exclusions))
	previous := ""
	for _, file := range snapshot.Files {
		if err := file.validate(snapshot.RepositoryID); err != nil {
			return err
		}
		if previous != "" && file.Path <= previous {
			return fmt.Errorf("repository snapshot files must be uniquely sorted by path")
		}
		previous = file.Path
		seen[file.Path] = struct{}{}
	}
	previous = ""
	for _, exclusion := range snapshot.Exclusions {
		if err := validateRelativeRepositoryPath(exclusion.Path); err != nil {
			return fmt.Errorf("repository exclusion path: %w", err)
		}
		if !validExclusionReason(exclusion.Reason) {
			return fmt.Errorf("repository exclusion %q has invalid reason %q", exclusion.Path, exclusion.Reason)
		}
		if previous != "" && exclusion.Path <= previous {
			return fmt.Errorf("repository snapshot exclusions must be uniquely sorted by path")
		}
		previous = exclusion.Path
		if _, exists := seen[exclusion.Path]; exists {
			return fmt.Errorf("repository path %q appears in both files and exclusions", exclusion.Path)
		}
		seen[exclusion.Path] = struct{}{}
	}
	return nil
}

func (file File) validate(repositoryID string) error {
	if err := validateRelativeRepositoryPath(file.Path); err != nil {
		return fmt.Errorf("repository file path: %w", err)
	}
	if file.ID != opaqueID("file_", repositoryID, file.Path) {
		return fmt.Errorf("repository file %q has an invalid opaque ID", file.Path)
	}
	if file.Kind != EntryRegular && file.Kind != EntrySymlink {
		return fmt.Errorf("repository file %q has unsupported kind %q", file.Path, file.Kind)
	}
	if !validSHA256(file.SHA256) || file.Size < 0 {
		return fmt.Errorf("repository file %q has invalid content identity", file.Path)
	}
	if file.Kind == EntryRegular && file.LinkTarget != "" {
		return fmt.Errorf("regular repository file %q cannot have a link target", file.Path)
	}
	if file.Kind == EntrySymlink && file.LinkTarget == "" {
		return fmt.Errorf("repository symlink %q requires its exact link target", file.Path)
	}
	return nil
}

func snapshotID(snapshot Snapshot) (string, error) {
	identity := snapshotIdentity{
		Schema: snapshot.Schema, RepositoryID: snapshot.RepositoryID,
		HeadCommit: snapshot.HeadCommit, GitStateSHA256: snapshot.GitStateSHA256,
		Files: snapshot.Files, Exclusions: snapshot.Exclusions,
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode repository snapshot identity: %w", err)
	}
	hash := sha256.Sum256(raw)
	return "snapshot_" + hex.EncodeToString(hash[:]), nil
}

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
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil && len(decoded) == sha256.Size
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	return err == nil && len(decoded) == sha256.Size
}

func validGitOID(value string) bool {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	return err == nil && (len(decoded) == 20 || len(decoded) == sha256.Size)
}

func validExclusionReason(reason ExclusionReason) bool {
	switch reason {
	case ExclusionSensitive, ExclusionAbsent, ExclusionUnsupported:
		return true
	default:
		return false
	}
}

func sortSnapshotFacts(snapshot *Snapshot) {
	sort.Slice(snapshot.Files, func(left, right int) bool {
		return snapshot.Files[left].Path < snapshot.Files[right].Path
	})
	sort.Slice(snapshot.Exclusions, func(left, right int) bool {
		return snapshot.Exclusions[left].Path < snapshot.Exclusions[right].Path
	})
}
