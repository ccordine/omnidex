package workspace

import (
	"fmt"
	"strings"
)

const (
	MutationPlanSchemaV1 = "omnidex.workspace-mutation-plan.v1"

	MaxMutationFiles      = 32
	MaxMutationFileBytes  = 128 * 1024
	MaxMutationPatchBytes = 32 * 1024 * 1024
)

// ExactSourceFile binds a desired path to the exact included entry from which
// code derived that desired state. Nil source authority means the path must be
// absent; it never means "use whatever is currently there".
type ExactSourceFile struct {
	EntryID string `json:"entry_id"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	Mode    uint32 `json:"mode"`
}

// DesiredFileState describes truth after the transaction. Operation labels
// are deliberately absent; code derives create, replace, delete, or no-op by
// comparing this state with one exact Snapshot.
type DesiredFileState struct {
	Path    string           `json:"path"`
	Source  *ExactSourceFile `json:"source,omitempty"`
	Present bool             `json:"present"`
	Content []byte           `json:"-"`
	Mode    uint32           `json:"mode"`
}

type MutationFileState struct {
	Present bool   `json:"present"`
	SHA256  string `json:"sha256,omitempty"`
	Size    int64  `json:"size"`
	Mode    uint32 `json:"mode"`
}

type MutationFileTransition struct {
	FileID   string            `json:"file_id"`
	Path     string            `json:"path"`
	Source   MutationFileState `json:"source"`
	Expected MutationFileState `json:"expected"`
}

type MutationPlan struct {
	Schema              string                   `json:"schema"`
	ID                  string                   `json:"id"`
	OwnerID             string                   `json:"owner_id"`
	WorkspaceID         string                   `json:"workspace_id"`
	WorkspaceRoot       string                   `json:"workspace_root"`
	SourceStateID       string                   `json:"source_state_id"`
	ExpectedStateID     string                   `json:"expected_state_id"`
	GitSourceSnapshotID string                   `json:"git_source_snapshot_id,omitempty"`
	GitRepositoryID     string                   `json:"git_repository_id,omitempty"`
	GitHeadCommit       string                   `json:"git_head_commit,omitempty"`
	Patch               string                   `json:"patch"`
	PatchSHA256         string                   `json:"patch_sha256"`
	Files               []MutationFileTransition `json:"files"`
}

func (state MutationFileState) validate(label, path string) error {
	if !state.Present {
		if state.SHA256 != "" || state.Size != 0 || state.Mode != 0 {
			return fmt.Errorf("workspace mutation %s state for %q has authority for an absent file", label, path)
		}
		return nil
	}
	if !validSHA256(state.SHA256) || state.Size < 0 || state.Size > MaxMutationFileBytes ||
		state.Mode == 0 || state.Mode&^uint32(0o777) != 0 {
		return fmt.Errorf("workspace mutation %s state for %q is invalid", label, path)
	}
	return nil
}

func validMutationOwnerID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 256 ||
		strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	separator := len(value) - 65
	if separator < 1 || separator > 64 || value[separator] != '_' || !validSHA256(value[separator+1:]) {
		return false
	}
	for index, char := range value[:separator] {
		if char >= 'a' && char <= 'z' || index > 0 && (char >= '0' && char <= '9' || char == '_') {
			continue
		}
		return false
	}
	return true
}
