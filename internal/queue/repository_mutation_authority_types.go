package queue

import (
	"context"

	"github.com/gryph/omnidex/internal/model"
)

const (
	maxRepositoryMutationPatchBytes = 1 << 20
	maxRepositoryMutationFiles      = 8
)

// RepositoryMutationFile binds one changed path to exact source and expected
// presence states. Paths are retained only as server-owned evidence; they are
// never model authority. Presence transitions describe repository truth, not
// model-selectable filesystem operations.
type RepositoryMutationFile struct {
	FileID          string `json:"file_id"`
	Path            string `json:"path"`
	SourcePresent   bool   `json:"source_present"`
	SourceSHA256    string `json:"source_sha256,omitempty"`
	SourceSize      int64  `json:"source_size"`
	SourceMode      uint32 `json:"source_mode"`
	ExpectedPresent bool   `json:"expected_present"`
	ExpectedSHA256  string `json:"expected_sha256,omitempty"`
	ExpectedSize    int64  `json:"expected_size"`
	ExpectedMode    uint32 `json:"expected_mode"`
}

// RepositoryMutationCommand binds one filesystem callback to the exact
// running claim and immutable generated patch that may be accepted.
type RepositoryMutationCommand struct {
	JobID            int64                    `json:"job_id"`
	StepID           int64                    `json:"step_id"`
	Generation       int64                    `json:"generation"`
	Attempt          int64                    `json:"attempt"`
	WorkerID         string                   `json:"worker_id"`
	ContractID       string                   `json:"contract_id"`
	StageID          string                   `json:"stage_id"`
	SourceSnapshotID string                   `json:"source_snapshot_id"`
	Patch            string                   `json:"patch"`
	PatchSHA256      string                   `json:"patch_sha256"`
	ChangedFiles     []RepositoryMutationFile `json:"changed_files"`
}

func (command RepositoryMutationCommand) stepAttemptAuthority() model.StepAttemptAuthority {
	return model.StepAttemptAuthority{
		JobID: command.JobID, Generation: command.Generation, StepID: command.StepID,
		Attempt: command.Attempt, WorkerID: command.WorkerID,
	}
}

// RepositoryMutationState is a code-derived classification of the complete
// current repository inventory against one immutable source and post state.
type RepositoryMutationState string

const (
	RepositoryMutationSource        RepositoryMutationState = "source"
	RepositoryMutationPost          RepositoryMutationState = "post"
	RepositoryMutationIndeterminate RepositoryMutationState = "indeterminate"
)

// RepositoryMutationClassifier must compare the complete repository
// inventory, not only the files named by the mutation command.
type RepositoryMutationClassifier func(
	context.Context,
	RepositoryMutationCommand,
) (RepositoryMutationState, error)

type repositoryMutationOperationRecord struct {
	ID            string
	CommandSHA256 string
	Status        string
	EvidenceID    *int64
}

type repositoryMutationOperationIdentity struct {
	ID            string
	CommandSHA256 string
}

const (
	repositoryMutationPrepared      = "prepared"
	repositoryMutationApplying      = "applying"
	repositoryMutationApplied       = "applied"
	repositoryMutationIndeterminate = "indeterminate"
)
