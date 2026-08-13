package changeapply

import (
	"context"
	"fmt"
	"sync"

	"github.com/gryph/omnidex/internal/omni"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

const (
	maxCandidateDeclarationBytes = 64 * 1024
	maxTargetFileBytes           = 16 * 1024 * 1024
)

// CandidateDeclaration is the path-free output accepted from one bounded
// declaration worker. Filesystem authority remains inside repository facts.
type CandidateDeclaration struct {
	SymbolID    string `json:"symbol_id"`
	Declaration string `json:"declaration"`
}

type Input struct {
	Snapshot   repositoryfacts.Snapshot
	Analysis   repositoryfacts.Analysis
	Contract   repositoryfacts.ChangeContract
	Candidates []CandidateDeclaration
}

// ExactSourceFile is the immutable source-side authority for a desired absent
// state. It is constructed by code from an indexed snapshot and is never part
// of a model contract.
type ExactSourceFile struct {
	FileID string `json:"file_id"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}

// DesiredFileState states repository truth, not an operation. Code compares it
// with Snapshot and derives the one exact physical transition. Present source
// bytes are already mechanically assembled around bounded declarations.
type DesiredFileState struct {
	Path              string          `json:"path"`
	Present           bool            `json:"present"`
	Source            ExactSourceFile `json:"source"`
	Content           []byte          `json:"-"`
	Mode              uint32          `json:"mode"`
	PackageArtifactID string          `json:"package_artifact_id,omitempty"`
	RemovedSymbolIDs  []string        `json:"removed_symbol_ids,omitempty"`
}

type FileStateInput struct {
	Snapshot repositoryfacts.Snapshot
	Analysis repositoryfacts.Analysis
	OwnerID  string
	Desired  []DesiredFileState
}

type targetReplacement struct {
	symbolID    string
	fileID      string
	start       int64
	end         int64
	expected    string
	declaration []byte
}

type fileMutation struct {
	file           repositoryfacts.File
	original       []byte
	next           []byte
	replacements   []targetReplacement
	sourcePresent  bool
	desiredPresent bool
}

// ExpectedFileState is the exact post-patch authority for one changed file.
// Callers must compare these bytes with the final authoritative repository;
// merely observing that a file changed is not sufficient proof.
type ExpectedFileState struct {
	FileID  string `json:"file_id"`
	Path    string `json:"path"`
	Present bool   `json:"present"`
	SHA256  string `json:"sha256,omitempty"`
	Size    int64  `json:"size"`
	Mode    uint32 `json:"mode"`
}

type stagedFileAuthority struct {
	path       string
	kind       repositoryfacts.EntryKind
	sha256     string
	size       int64
	mode       uint32
	linkTarget string
}

type StagedChange struct {
	mu sync.Mutex

	id                 string
	workspace          string
	authoritativeRoot  string
	expectedSnapshotID string
	patch              string
	patchSHA256        string
	changedFileIDs     []string
	expectedFiles      []ExpectedFileState
	stagedFiles        []stagedFileAuthority
	closed             bool
	applied            bool
}

func (stage *StagedChange) ID() string {
	if stage == nil {
		return ""
	}
	return stage.id
}

func (stage *StagedChange) Workspace() string {
	if stage == nil {
		return ""
	}
	return stage.workspace
}

func (stage *StagedChange) Patch() string {
	if stage == nil {
		return ""
	}
	return stage.patch
}

func (stage *StagedChange) PatchSHA256() string {
	if stage == nil {
		return ""
	}
	return stage.patchSHA256
}

func (stage *StagedChange) ChangedFileIDs() []string {
	if stage == nil {
		return nil
	}
	return append([]string(nil), stage.changedFileIDs...)
}

func (stage *StagedChange) ExpectedFiles() []ExpectedFileState {
	if stage == nil {
		return nil
	}
	return append([]ExpectedFileState(nil), stage.expectedFiles...)
}

func (stage *StagedChange) ApplyVerified(ctx context.Context) (omni.PatchApplyResult, error) {
	if stage == nil {
		return omni.PatchApplyResult{}, fmt.Errorf("apply verified repository change requires a staged change")
	}
	return stage.applyVerified(ctx)
}
