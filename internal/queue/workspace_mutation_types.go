package queue

import (
	"context"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

const (
	workspaceMutationCommandSchema      = "omnidex.workspace-mutation-command.v1"
	WorkspaceMutationVerificationPlanV1 = "omnidex.workspace-mutation-verification-plan.v1"
	workspaceMutationReceiptSchema      = "omnidex.workspace-mutation-verification-receipt.v1"
)

type WorkspaceMutationVerificationIntent struct {
	Kind    string
	Command string
}

type WorkspaceMutationVerificationCommand struct {
	Ordinal       int    `json:"ordinal"`
	Kind          string `json:"kind"`
	Command       string `json:"command"`
	CommandSHA256 string `json:"command_sha256"`
}

type WorkspaceMutationVerificationPlan struct {
	Schema   string                                 `json:"schema"`
	Commands []WorkspaceMutationVerificationCommand `json:"commands"`
}

// WorkspaceMutationCommand binds one code-derived workspace delta to the
// exact claim that created it. Recovery may run under a later active attempt;
// creator authority remains immutable in this command.
type WorkspaceMutationCommand struct {
	JobID           int64  `json:"job_id"`
	StepID          int64  `json:"step_id"`
	Generation      int64  `json:"generation"`
	CreatorAttempt  int64  `json:"creator_attempt"`
	CreatorWorkerID string `json:"creator_worker_id"`
	ProjectID       int64  `json:"project_id"`
	// ProjectLocation is host-visible execution authority persisted outside
	// the historical v1 command JSON and its immutable digest.
	ProjectLocation string                            `json:"-"`
	Plan            workspacefacts.MutationPlan       `json:"plan"`
	Verification    WorkspaceMutationVerificationPlan `json:"verification"`
}

func (command WorkspaceMutationCommand) creatorAuthority() model.StepAttemptAuthority {
	return model.StepAttemptAuthority{
		JobID: command.JobID, Generation: command.Generation, StepID: command.StepID,
		Attempt: command.CreatorAttempt, WorkerID: command.CreatorWorkerID,
	}
}

type WorkspaceMutationObservation string

const (
	WorkspaceMutationSource        WorkspaceMutationObservation = "source"
	WorkspaceMutationPost          WorkspaceMutationObservation = "post"
	WorkspaceMutationIndeterminate WorkspaceMutationObservation = "indeterminate"
)

type WorkspaceMutationVerificationResult struct {
	Succeeded                    bool
	Failure                      string
	CommandEvidence              []evidence.Record
	VerifiedRepositorySnapshotID string
}

type WorkspaceMutationCallbacks struct {
	Observe func(context.Context, WorkspaceMutationCommand) (WorkspaceMutationObservation, error)
	Apply   func(context.Context, WorkspaceMutationCommand) error
	Verify  func(context.Context, WorkspaceMutationCommand) (WorkspaceMutationVerificationResult, error)
}

type WorkspaceMutationResult struct {
	OperationID                  string
	Status                       string
	MutationEvidenceID           int64
	CommandEvidenceIDs           []int64
	VerificationEvidenceID       int64
	VerificationSucceeded        bool
	VerifiedRepositorySnapshotID string
}

// WorkspaceMutationTerminal is the immutable terminal authority sealed by one
// workspace mutation operation. ReceiptJSON is preserved byte-for-byte and
// bound to ReceiptSHA256.
type WorkspaceMutationTerminal struct {
	Result        WorkspaceMutationResult
	Failure       string
	ReceiptJSON   string
	ReceiptSHA256 string
}

// WorkspaceMutationSnapshot is the one exact operation belonging to a job's
// current generation. Terminal is nil until verification has been sealed.
type WorkspaceMutationSnapshot struct {
	OperationID string
	Status      string
	Command     WorkspaceMutationCommand
	Terminal    *WorkspaceMutationTerminal
}

type workspaceMutationOperationIdentity struct {
	ID              string
	CommandSHA256   string
	ProjectLocation string
	PlanJSON        string
	PlanSHA256      string
}

type workspaceMutationOperationRecord struct {
	ID                           string
	CommandSHA256                string
	ProjectLocation              string
	Status                       string
	IndeterminatePhase           *string
	MutationEvidenceID           *int64
	VerificationSucceeded        *bool
	VerificationReceipt          *string
	VerificationEvidenceID       *int64
	VerifiedRepositorySnapshotID *string
}

const (
	workspaceMutationPrepared           = "prepared"
	workspaceMutationApplying           = "applying"
	workspaceMutationApplied            = "applied"
	workspaceMutationVerifying          = "verifying"
	workspaceMutationVerified           = "verified"
	workspaceMutationVerificationFailed = "verification_failed"
	workspaceMutationIndeterminateState = "indeterminate"
)
