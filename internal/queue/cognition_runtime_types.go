package queue

import (
	"errors"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	cognitionEpisodeSchemaV1       = "omnidex.cognition-episode.v1"
	cognitionQueueIdentitySchemaV1 = "omnidex.cognition-queue-authority.v1"
	cognitionObligationNodePrefix  = "cognition_obligation_"
	cognitionObligationEdgePrefix  = "cognition_edge_"
	cognitionEntryPrefix           = "cognition_entry_"
	cognitionTransitionPrefix      = "cognition_transition_"
)

var (
	ErrCognitionEpisodeNotFound = errors.New("cognition episode not found")
	ErrCognitionActionNotFound  = errors.New("cognition action not found")
	ErrCognitionConflict        = errors.New("cognition authority conflict")
	ErrCognitionTerminal        = errors.New("cognition episode is terminal")
	ErrCognitionBudgetExhausted = errors.New("cognition policy budget exhausted")
	ErrCognitionEnvelopeBudget  = errors.New("cognition policy envelope budget exceeded")
)

type CognitionEpisodeStart struct {
	Authority     model.StepAttemptAuthority
	EpisodeID     cognition.EpisodeID
	AttestedBrain cognitionpolicy.AttestedBrain
	Scenario      cognition.ScenarioRef
	Goal          cognition.GoalExpression
	Completion    cognition.CompletionAuthority
	ActionCatalog cognition.ActionCatalog
	Budget        cognition.RuntimeBudget
	Root          cognition.ObligationSpec
	Transition    cognition.Transition
}

type CognitionEpisode struct {
	EpisodeID             cognition.EpisodeID
	Authority             model.StepAttemptAuthority
	AttestedBrain         cognitionpolicy.AttestedBrain
	FactAuthority         cognitionstate.FactAcceptanceAuthorityRef
	LedgerID              taskstate.LedgerID
	WorkingSetID          string
	Scenario              cognition.ScenarioRef
	Goal                  cognition.GoalExpression
	Completion            cognition.CompletionAuthority
	ActionCatalog         cognition.ActionCatalog
	Budget                cognition.RuntimeBudget
	CurrentRevision       cognition.WorldRevision
	Status                CognitionEpisodeStatus
	SuccessfulActions     int64
	TotalCost             int64
	Version               uint64
	TerminalOutcome       string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CreatedAttemptExpires time.Time
}

type CognitionEpisodeStatus string

const (
	CognitionEpisodeActive    CognitionEpisodeStatus = "active"
	CognitionEpisodeCompleted CognitionEpisodeStatus = "completed"
	CognitionEpisodeFailed    CognitionEpisodeStatus = "failed"
	CognitionEpisodeCanceled  CognitionEpisodeStatus = "canceled"
)

type CognitionActionRecord struct {
	EpisodeID            cognition.EpisodeID
	Action               cognition.RegisteredAction
	ObligationID         cognition.ObligationID
	PolicyCallID         string
	ReconciliationID     string
	ReconciliationSHA256 string
	ExpectedRevision     cognition.WorldRevision
	SnapshotSHA256       string
	ContextProjection    cognition.ContextProjectionRef
	Decision             cognition.CognitionDecision
	Schema               cognition.ActionSchemaRef
	Status               CognitionActionStatus
	Failure              *cognition.ActionFailure
	ResultRevision       *cognition.WorldRevision
	Origin               model.StepAttemptAuthority
	CreatedAt            time.Time
	DispatchedAt         *time.Time
	ResolvedAt           *time.Time
}

func (record CognitionActionRecord) ActionFor(
	authority model.StepAttemptAuthority,
) (cognition.RegisteredAction, error) {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return cognition.RegisteredAction{}, err
	}
	if authority.JobID != record.Origin.JobID || authority.Generation != record.Origin.Generation ||
		authority.StepID != record.Origin.StepID {
		return cognition.RegisteredAction{}, staleStepAttemptError(authority, "cognition action belongs to another step", nil)
	}
	action := record.Action
	action.Actor = cognition.AttemptRef{
		JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		Attempt: uint64(authority.Attempt), WorkerID: authority.WorkerID,
	}
	return action, nil
}

type CognitionActionStatus string

const (
	CognitionActionPrepared   CognitionActionStatus = "prepared"
	CognitionActionDispatched CognitionActionStatus = "dispatched"
	CognitionActionSucceeded  CognitionActionStatus = "succeeded"
	CognitionActionFailed     CognitionActionStatus = "failed"
)

type CognitionTerminalCommand struct {
	Authority        model.StepAttemptAuthority
	EpisodeID        cognition.EpisodeID
	Outcome          CognitionEpisodeStatus
	GraphVersion     uint64
	Completion       cognition.CompletionResult
	ObligationGraph  cognition.ObligationGraphSnapshot
	PublicOutcome    string
	ExpectedRevision cognition.WorldRevision
}

type CognitionTerminalSeal struct {
	EpisodeID             cognition.EpisodeID
	Outcome               CognitionEpisodeStatus
	FinalRevision         cognition.WorldRevision
	CompletionSHA256      string
	ObligationGraphSHA256 string
	LedgerVersion         uint64
	WorkingSetVersion     uint64
	TraceSHA256           string
	AuthorityKind         string
	SealedBy              model.StepAttemptAuthority
	LifecycleOperationID  LifecycleOperationID
	CreatedAt             time.Time
}

type CognitionObligationGraphRecord struct {
	EpisodeID     cognition.EpisodeID
	Version       uint64
	CommandID     string
	CommandSHA256 string
	CommandKind   CognitionObligationCommandKind
	Graph         cognition.ObligationGraphSnapshot
	Actor         model.StepAttemptAuthority
	CreatedAt     time.Time
}
