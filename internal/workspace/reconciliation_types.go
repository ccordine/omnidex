package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	ReconciliationPlanSchemaV1 = "omnidex.workspace-reconciliation-plan.v1"
	MaxReconciliationFiles     = 4096
	MaxReconciliationFileBytes = 32 * 1024 * 1024
	MaxReconciliationTotalBytes = 256 * 1024 * 1024
)

// DesiredFile is one path's requested filesystem truth. Present distinguishes
// an empty regular file from an absent path; Content is otherwise byte-opaque.
type DesiredFile struct {
	Path    string `json:"path"`
	Present bool   `json:"present"`
	Content []byte `json:"content,omitempty"`
	Mode    uint32 `json:"mode"`
}

type ReconciliationFileState struct {
	Present    bool      `json:"present"`
	Kind       EntryKind `json:"kind,omitempty"`
	SHA256     string    `json:"sha256,omitempty"`
	Size       int64     `json:"size"`
	Mode       uint32    `json:"mode"`
	LinkTarget string    `json:"link_target,omitempty"`
}

type ReconciliationFileTransition struct {
	Path     string                  `json:"path"`
	Source   ReconciliationFileState `json:"source"`
	Expected ReconciliationFileState `json:"expected"`
	Content  []byte                  `json:"content,omitempty"`
}

type ReconciliationMove struct {
	SourcePath      string `json:"source_path"`
	DestinationPath string `json:"destination_path"`
}

type ReconciliationPlan struct {
	Schema          string                         `json:"schema"`
	ID              string                         `json:"id"`
	OwnerID         string                         `json:"owner_id"`
	WorkspaceID     string                         `json:"workspace_id"`
	Root            string                         `json:"root"`
	SourceStateID   string                         `json:"source_state_id"`
	ExpectedStateID string                         `json:"expected_state_id"`
	Paths           []string                       `json:"paths"`
	Files           []ReconciliationFileTransition `json:"files"`
	Moves           []ReconciliationMove           `json:"moves"`
}

type ReconciliationResult struct {
	PlanID          string   `json:"plan_id"`
	SourceStateID   string   `json:"source_state_id"`
	ExpectedStateID string   `json:"expected_state_id"`
	ChangedPaths    []string `json:"changed_paths"`
	Moves           []ReconciliationMove `json:"moves"`
	Snapshot        Snapshot `json:"snapshot"`
}

func (state ReconciliationFileState) validate(label, relative string) error {
	if !state.Present {
		if state.Kind != "" || state.SHA256 != "" || state.Size != 0 || state.Mode != 0 ||
			state.LinkTarget != "" {
			return fmt.Errorf("workspace reconciliation %s state for %q contains absent-path authority", label, relative)
		}
		return nil
	}
	if !validSHA256(state.SHA256) || state.Size < 0 || state.Mode&^uint32(0o777) != 0 {
		return fmt.Errorf("workspace reconciliation %s state for %q is invalid", label, relative)
	}
	switch state.Kind {
	case EntryRegular:
		if state.LinkTarget != "" {
			return fmt.Errorf("workspace reconciliation %s regular state for %q has a link target", label, relative)
		}
	case EntrySymlink:
		if state.LinkTarget == "" || int64(len(state.LinkTarget)) != state.Size {
			return fmt.Errorf("workspace reconciliation %s symlink state for %q is invalid", label, relative)
		}
	case EntryDirectory:
		if state.LinkTarget != "" || state.Size != 0 || state.SHA256 != digestBytes([]byte("directory\x00")) {
			return fmt.Errorf("workspace reconciliation %s directory state for %q is invalid", label, relative)
		}
	default:
		return fmt.Errorf("workspace reconciliation %s state for %q has unsupported kind %q", label, relative, state.Kind)
	}
	return nil
}

func reconciliationState(entry Entry) ReconciliationFileState {
	if entry.Path == "" {
		return ReconciliationFileState{}
	}
	return ReconciliationFileState{
		Present: true, Kind: entry.Kind, SHA256: entry.SHA256, Size: entry.Size,
		Mode: entry.Mode, LinkTarget: entry.LinkTarget,
	}
}

func digestBytes(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func validReconciliationOwnerID(value string) bool {
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

func cloneReconciliationPlan(source ReconciliationPlan) ReconciliationPlan {
	clone := source
	clone.Paths = append([]string(nil), source.Paths...)
	clone.Files = make([]ReconciliationFileTransition, len(source.Files))
	for index, transition := range source.Files {
		clone.Files[index] = transition
		clone.Files[index].Content = append([]byte(nil), transition.Content...)
	}
	clone.Moves = make([]ReconciliationMove, len(source.Moves))
	copy(clone.Moves, source.Moves)
	return clone
}

func cloneSnapshot(source Snapshot) Snapshot {
	clone := source
	clone.Paths = append([]string(nil), source.Paths...)
	clone.Entries = append([]Entry(nil), source.Entries...)
	return clone
}
