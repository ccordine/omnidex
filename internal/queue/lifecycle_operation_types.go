package queue

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
)

var (
	ErrLifecycleOperationConflict = errors.New("lifecycle operation identity conflict")
	lifecycleOperationIDPattern   = regexp.MustCompile(`^lifecycle_operation_[a-z0-9_]{1,128}$`)
)

type LifecycleOperationID string

func NewLifecycleOperationID() (LifecycleOperationID, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("generate lifecycle operation identity: %w", err)
	}
	return LifecycleOperationID("lifecycle_operation_" + hex.EncodeToString(nonce[:])), nil
}

// A worker retry belongs to the same persisted step attempt. Its ID does not
// depend on output text or require allocating another operation on retry.
func NewStepLifecycleOperationID(authority model.StepAttemptAuthority, kind LifecycleOperationKind) (LifecycleOperationID, error) {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return "", err
	}
	if kind != LifecycleCompleteStep && kind != LifecycleFailStep {
		return "", fmt.Errorf("step lifecycle operation kind %q is unsupported", kind)
	}
	return ParseLifecycleOperationID(fmt.Sprintf(
		"lifecycle_operation_step_%d_attempt_%d_%s", authority.StepID, authority.Attempt, kind,
	))
}

func ParseLifecycleOperationID(value string) (LifecycleOperationID, error) {
	id := LifecycleOperationID(value)
	if !lifecycleOperationIDPattern.MatchString(value) {
		return "", fmt.Errorf("lifecycle operation ID requires lifecycle_operation_ plus 1..128 lowercase letters, digits, or underscores")
	}
	return id, nil
}

type LifecycleOperationKind string

const (
	LifecycleCompleteStep        LifecycleOperationKind = "complete_step"
	LifecycleFailStep            LifecycleOperationKind = "fail_step"
	LifecycleSubmitFeedback      LifecycleOperationKind = "submit_feedback"
	LifecycleInterruptJob        LifecycleOperationKind = "interrupt_job"
	LifecycleReplanJob           LifecycleOperationKind = "replan_job"
	LifecycleChannelSession      LifecycleOperationKind = "channel_session_turn"
	LifecycleScrumChannel        LifecycleOperationKind = "scrum_channel_message"
	LifecycleCancelJob           LifecycleOperationKind = "cancel_job"
	LifecycleCodingPlanDecisions LifecycleOperationKind = "coding_plan_decisions"
	LifecycleCodingPlanFreeze    LifecycleOperationKind = "coding_plan_freeze"
)

type CompleteStepCommand struct {
	OperationID               LifecycleOperationID                 `json:"operation_id"`
	Authority                 model.StepAttemptAuthority           `json:"-"`
	StepID                    int64                                `json:"step_id"`
	Output                    string                               `json:"output"`
	ContextKey                string                               `json:"context_key,omitempty"`
	RoleplayResponses         []RoleplayResponseCompletion         `json:"roleplay_responses,omitempty"`
	RoleplayUserCanon         *RoleplayUserCanonCompletion         `json:"roleplay_user_canon,omitempty"`
	RoleplayUserOngoingAction *RoleplayUserOngoingActionCompletion `json:"roleplay_user_ongoing_action,omitempty"`
}

type RoleplayResponseCompletion struct {
	Position              int                         `json:"position"`
	CharacterID           model.RoleplayCharacterID   `json:"character_id"`
	Output                string                      `json:"output"`
	Facts                 []string                    `json:"facts"`
	KnowledgeCharacterIDs []model.RoleplayCharacterID `json:"knowledge_character_ids"`
	PreviousOngoingAction *string                     `json:"previous_ongoing_action,omitempty"`
	OngoingAction         *string                     `json:"ongoing_action,omitempty"`
}

type RoleplayUserOngoingActionCompletion struct {
	CharacterID           model.RoleplayCharacterID `json:"character_id"`
	PreviousOngoingAction *string                   `json:"previous_ongoing_action"`
	OngoingAction         *string                   `json:"ongoing_action"`
}

type RoleplayUserCanonCompletion struct {
	Facts                 []string                    `json:"facts"`
	KnowledgeCharacterIDs []model.RoleplayCharacterID `json:"knowledge_character_ids"`
}

// CompleteStepEvidenceCommand binds the complete objective citation set to the
// same immutable lifecycle operation that completes the step. Objective
// citations are not writable through the generic evidence sidecar.
type CompleteStepEvidenceCommand struct {
	CompleteStepCommand
	Evidence []evidence.Record `json:"evidence"`
}

type FailStepCommand struct {
	OperationID LifecycleOperationID       `json:"operation_id"`
	Authority   model.StepAttemptAuthority `json:"-"`
	StepID      int64                      `json:"step_id"`
	Error       string                     `json:"error"`
}

type SubmitJobFeedbackCommand struct {
	OperationID       LifecycleOperationID `json:"operation_id"`
	JobID             int64                `json:"job_id"`
	Feedback          string               `json:"feedback"`
	WorkspaceRoot     string               `json:"workspace_root,omitempty"`
	WorkspaceIdentity string               `json:"workspace_identity,omitempty"`
}

type ReplanJobCommand struct {
	OperationID       LifecycleOperationID `json:"operation_id"`
	JobID             int64                `json:"job_id"`
	Feedback          string               `json:"feedback"`
	WorkspaceRoot     string               `json:"workspace_root,omitempty"`
	WorkspaceIdentity string               `json:"workspace_identity,omitempty"`
}

type CancelJobCommand struct {
	OperationID       LifecycleOperationID `json:"operation_id"`
	JobID             int64                `json:"job_id"`
	Reason            string               `json:"reason"`
	WorkspaceRoot     string               `json:"workspace_root,omitempty"`
	WorkspaceIdentity string               `json:"workspace_identity,omitempty"`
}

// LifecycleJobResult distinguishes a newly committed mutation from the exact
// immutable receipt returned for an idempotent operation replay.
type LifecycleJobResult struct {
	Job     model.Job
	Applied bool
}
