package queue

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
)

const lifecycleOperationIdentitySchema = "omnidex.lifecycle-operation-identity.v1"

var (
	ErrLifecycleOperationConflict = errors.New("lifecycle operation identity conflict")
	lifecycleOperationIDPattern   = regexp.MustCompile(`^lifecycle_operation_[0-9a-f]{64}$`)
)

type LifecycleOperationID string

func NewLifecycleOperationID(parts ...string) (LifecycleOperationID, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("lifecycle operation identity requires at least one part")
	}
	for index, part := range parts {
		if part == "" || part != strings.TrimSpace(part) {
			return "", fmt.Errorf("lifecycle operation identity part %d must be one nonempty exact value", index)
		}
	}
	return LifecycleOperationID("lifecycle_operation_" + lifecycleIdentityDigest(
		append([]string{lifecycleOperationIdentitySchema}, parts...)...,
	)), nil
}

func NewRandomLifecycleOperationID() (LifecycleOperationID, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("generate lifecycle operation identity: %w", err)
	}
	return NewLifecycleOperationID("random", hex.EncodeToString(nonce[:]))
}

func ParseLifecycleOperationID(value string) (LifecycleOperationID, error) {
	id := LifecycleOperationID(value)
	if !lifecycleOperationIDPattern.MatchString(value) {
		return "", fmt.Errorf("lifecycle operation ID must match lifecycle_operation_ plus 64 lowercase hex characters")
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

func lifecycleIdentityDigest(parts ...string) string {
	var canonical bytes.Buffer
	for _, part := range parts {
		_ = binary.Write(&canonical, binary.BigEndian, uint64(len(part)))
		_, _ = canonical.WriteString(part)
	}
	digest := sha256.Sum256(canonical.Bytes())
	return hex.EncodeToString(digest[:])
}
