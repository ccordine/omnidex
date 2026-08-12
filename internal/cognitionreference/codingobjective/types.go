package codingobjective

import (
	"context"
	"errors"

	"github.com/gryph/omnidex/internal/assemblyline"
)

var (
	ErrInvalidObjective   = errors.New("invalid reference coding objective")
	ErrAlreadySatisfied   = errors.New("reference repair objective is already satisfied")
	ErrRepositoryEvidence = errors.New("invalid reference repository evidence")
	ErrDeclaration        = errors.New("invalid reference declaration")
	ErrVerification       = errors.New("reference coding verification failed")
)

type AcceptancePredicate string

const AcceptanceGoTestsPass AcceptancePredicate = "go_tests_pass"

type CommitOutcome string

const (
	CommitNotAttempted CommitOutcome = "not_attempted"
	CommitSucceeded    CommitOutcome = "succeeded"
	CommitUnknown      CommitOutcome = "unknown"
)

type Step string

const (
	StepRepositorySnapshotted Step = "repository_snapshotted"
	StepRepositoryAnalyzed    Step = "repository_analyzed"
	StepTargetResolved        Step = "target_resolved"
	StepTargetObserved        Step = "target_observed"
	StepContractBuilt         Step = "contract_built"
	StepBaselineUnsatisfied   Step = "baseline_unsatisfied"
	StepDeclarationDispatched Step = "declaration_dispatched"
	StepDeclarationAccepted   Step = "declaration_accepted"
	StepChangeStaged          Step = "change_staged"
	StepStageFormatVerified   Step = "stage_format_verified"
	StepStageTestsVerified    Step = "stage_tests_verified"
	StepAuthoritativeApplied  Step = "authoritative_applied"
	StepObjectiveCompleted    Step = "objective_completed"
)

// Objective is code-held authority. Root and target identity are never part of
// the declaration station envelope.
type Objective struct {
	ID               string
	Root             string
	Target           string
	RequirementQuote string
	Acceptance       []AcceptancePredicate
}

// DeclarationStation fills one source-code leaf. It cannot select repository
// mechanics, receive filesystem authority, or declare completion.
type DeclarationStation interface {
	Generate(context.Context, assemblyline.PortableJob) (assemblyline.PortableResult, error)
}

// Result reports one trusted-fixture primitive integration. It does not assert
// autonomy, real model generation, hostile-process containment, network
// containment, exact executable-byte authority, Git metadata, ignored bytes,
// or sensitive excluded bytes.
type Result struct {
	ObjectiveID string
	Acceptance  []AcceptancePredicate
	// Satisfied means the exact disposable candidate workspace passed the
	// bound acceptance predicate. Complete additionally requires a reconciled
	// authoritative commit.
	Satisfied          bool
	Steps              []Step
	Complete           bool
	ModelCalls         int
	BeforeSnapshotID   string
	DirectCapabilities int
	DirectTests        int
	PortableJobID      string
	StageID            string
	PatchSHA256        string
	ChangedFileIDs     []string
	ExpectedFiles      []ExpectedFile
	CommitOutcome      CommitOutcome
}

// ExpectedFile is path-free reconciliation authority retained even when the
// final commit outcome is unknown.
type ExpectedFile struct {
	FileID string
	SHA256 string
	Size   int64
}

func successfulStepSequence() []Step {
	return []Step{
		StepRepositorySnapshotted,
		StepRepositoryAnalyzed,
		StepTargetResolved,
		StepTargetObserved,
		StepContractBuilt,
		StepBaselineUnsatisfied,
		StepDeclarationDispatched,
		StepDeclarationAccepted,
		StepChangeStaged,
		StepStageFormatVerified,
		StepStageTestsVerified,
		StepAuthoritativeApplied,
		StepObjectiveCompleted,
	}
}
