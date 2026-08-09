package queue

import "context"

const (
	maxRepositoryMutationPatchBytes = 1 << 20
	maxRepositoryMutationFiles      = 8
)

// RepositoryMutationFile binds one changed file to its exact expected
// post-patch state. Paths are retained only as server-owned evidence; they are
// never model authority.
type RepositoryMutationFile struct {
	FileID         string `json:"file_id"`
	Path           string `json:"path"`
	SourceSHA256   string `json:"source_sha256"`
	SourceSize     int64  `json:"source_size"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ExpectedSize   int64  `json:"expected_size"`
}

// RepositoryMutationCommand binds one filesystem callback to the exact
// running claim and immutable generated patch that may be accepted.
type RepositoryMutationCommand struct {
	JobID            int64                    `json:"job_id"`
	StepID           int64                    `json:"step_id"`
	Generation       int64                    `json:"generation"`
	WorkerID         string                   `json:"worker_id"`
	ContractID       string                   `json:"contract_id"`
	StageID          string                   `json:"stage_id"`
	SourceSnapshotID string                   `json:"source_snapshot_id"`
	Patch            string                   `json:"patch"`
	PatchSHA256      string                   `json:"patch_sha256"`
	ChangedFiles     []RepositoryMutationFile `json:"changed_files"`
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
