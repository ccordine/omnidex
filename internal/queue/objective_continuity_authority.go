package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// MaxObjectiveSessionContextBytes is both the persisted admission bound and
// the active projection bound. A command which would cross it must fail in
// the mutation transaction; already accepted authority is therefore always
// readable in full and is never silently truncated.
const MaxObjectiveSessionContextBytes = 32 * 1024

type ObjectiveSessionTurnKind string

const (
	ObjectiveSessionFollowup     ObjectiveSessionTurnKind = "follow_up"
	ObjectiveSessionRedirect     ObjectiveSessionTurnKind = "redirect"
	ObjectiveSessionInterruption ObjectiveSessionTurnKind = "interruption"
	ObjectiveSessionCancellation ObjectiveSessionTurnKind = "cancellation"
)

type ObjectiveSessionTurnAuthority struct {
	OperationID LifecycleOperationID
	Kind        ObjectiveSessionTurnKind
	Text        string
	ContextText string
	Generation  int64
	CreatedAt   time.Time
}

type ObjectiveSessionContextAuthority struct {
	JobID                    int64
	InitialInstruction       string
	Turns                    []ObjectiveSessionTurnAuthority
	CurrentReplanOperationID LifecycleOperationID
}

type ObjectiveContinuityAuthority struct {
	Replan  *assemblyline.ObjectiveReplanAuthority
	Session *ObjectiveSessionContextAuthority
}

// ObjectiveContinuityAuthorities loads the current generation's exact replan
// feedback and verifies any immutable channel-turn binding.
func (r *Repository) ObjectiveContinuityAuthorities(
	ctx context.Context,
	job model.Job,
	expectedBoundary string,
) (ObjectiveContinuityAuthority, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity requires PostgreSQL and context")
	}
	if job.ID < 1 || job.CurrentGeneration < 1 {
		return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity requires one positive current job generation")
	}
	switch expectedBoundary {
	case replanCodingBoundary, replanObjectiveBoundary:
	default:
		return ObjectiveContinuityAuthority{}, fmt.Errorf(
			"objective continuity boundary %q is not registered",
			expectedBoundary,
		)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	defer tx.Rollback(ctx)
	var stored model.Job
	if err := tx.QueryRow(ctx, `
		SELECT id,instruction,pipeline,status,metadata,current_generation
		FROM jobs WHERE id=$1
	`, job.ID).Scan(
		&stored.ID, &stored.Instruction, &stored.Pipeline, &stored.Status, &stored.Metadata,
		&stored.CurrentGeneration,
	); err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	if stored.Instruction != job.Instruction || stored.Pipeline != job.Pipeline ||
		stored.Status != job.Status ||
		stored.CurrentGeneration != job.CurrentGeneration {
		return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity job authority changed before projection")
	}
	providedBinding, providedExists, err := channelBindingForJob(job)
	if err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	storedBinding, storedExists, err := channelBindingForJob(stored)
	if err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	if providedExists != storedExists || !providedBinding.equal(storedBinding) {
		return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity channel binding differs from durable job authority")
	}
	authority := ObjectiveContinuityAuthority{}
	if stored.CurrentGeneration > 1 {
		var purpose, boundaryAction, feedback, feedbackSHA string
		if err := tx.QueryRow(ctx, `
			SELECT purpose,boundary_action,feedback,feedback_sha256
			FROM job_generations
			WHERE job_id=$1 AND generation=$2
		`, stored.ID, stored.CurrentGeneration).Scan(
			&purpose, &boundaryAction, &feedback, &feedbackSHA,
		); err != nil {
			return ObjectiveContinuityAuthority{}, err
		}
		if purpose != jobGenerationPurposeReplan || boundaryAction != expectedBoundary ||
			len(feedback) > assemblyline.MaxObjectiveReplanFeedbackBytes {
			return ObjectiveContinuityAuthority{}, fmt.Errorf(
				"current objective generation has invalid or oversized exact replan feedback",
			)
		}
		digest := sha256.Sum256([]byte(feedback))
		if feedbackSHA != hex.EncodeToString(digest[:]) {
			return ObjectiveContinuityAuthority{}, fmt.Errorf("current objective generation feedback hash does not match")
		}
		authority.Replan = &assemblyline.ObjectiveReplanAuthority{
			JobID: stored.ID, Generation: stored.CurrentGeneration,
			Feedback: feedback, FeedbackSHA256: feedbackSHA,
		}
		if err := authority.ReplanContext().Validate(); err != nil {
			return ObjectiveContinuityAuthority{}, err
		}
	} else {
		var exactInitial bool
		if err := tx.QueryRow(ctx, `
			SELECT purpose='initial' AND feedback IS NULL AND feedback_sha256 IS NULL
			FROM job_generations WHERE job_id=$1 AND generation=1
		`, stored.ID).Scan(&exactInitial); err != nil {
			return ObjectiveContinuityAuthority{}, err
		}
		if !exactInitial {
			return ObjectiveContinuityAuthority{}, fmt.Errorf("initial objective generation authority is malformed")
		}
	}
	if providedExists {
		var content string
		if err := tx.QueryRow(ctx, `
			SELECT message.content
			FROM ai_channels AS channel
			JOIN ai_channel_messages AS message
			  ON message.channel_id=channel.id
			WHERE channel.id=$1 AND channel.scope='user'
			  AND message.id=$2 AND message.role='user'
		`, providedBinding.ChannelID, providedBinding.UserMessageID).Scan(
			&content,
		); err != nil {
			return ObjectiveContinuityAuthority{}, err
		}
		if content != stored.Instruction {
			return ObjectiveContinuityAuthority{}, fmt.Errorf("objective continuity does not match exact channel turn authority")
		}
		if providedBinding.Mode == model.ChannelModeAssistant {
			session, err := loadObjectiveSessionContextTx(
				ctx,
				tx,
				stored,
				providedBinding.ChannelID,
				authority.Replan,
			)
			if err != nil {
				return ObjectiveContinuityAuthority{}, err
			}
			authority.Session = &session
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ObjectiveContinuityAuthority{}, err
	}
	return authority, nil
}

func loadObjectiveSessionContextTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	channelID string,
	replan *assemblyline.ObjectiveReplanAuthority,
) (ObjectiveSessionContextAuthority, error) {
	followups, err := loadConversationSessionFollowupsTx(ctx, tx, channelID, job.ID)
	if err != nil {
		return ObjectiveSessionContextAuthority{}, err
	}
	session := ObjectiveSessionContextAuthority{
		JobID:              job.ID,
		InitialInstruction: job.Instruction,
		Turns:              make([]ObjectiveSessionTurnAuthority, 0, len(followups)),
	}
	contextBytes := 0
	currentReplans := 0
	for _, followup := range followups {
		turn := ObjectiveSessionTurnAuthority{
			OperationID: followup.operationID,
			Kind:        ObjectiveSessionTurnKind(followup.kind),
			Text:        followup.text,
			ContextText: strings.TrimPrefix(followup.contextText, "\n\n"),
			Generation:  followup.generation,
			CreatedAt:   followup.createdAt,
		}
		if turn.ContextText == followup.contextText || strings.TrimSpace(turn.ContextText) == "" {
			return ObjectiveSessionContextAuthority{}, fmt.Errorf(
				"job %d session turn %q has invalid model context framing",
				job.ID,
				turn.OperationID,
			)
		}
		contextBytes += 2 + len(turn.ContextText)
		if contextBytes > MaxObjectiveSessionContextBytes {
			return ObjectiveSessionContextAuthority{}, fmt.Errorf(
				"job %d same-session context exceeds the explicit %d-byte bound",
				job.ID,
				MaxObjectiveSessionContextBytes,
			)
		}
		if replan != nil && turn.Kind == ObjectiveSessionRedirect &&
			turn.Generation == replan.Generation && turn.Text == replan.Feedback {
			currentReplans++
			session.CurrentReplanOperationID = turn.OperationID
		}
		session.Turns = append(session.Turns, turn)
	}
	if replan != nil && currentReplans != 1 {
		return ObjectiveSessionContextAuthority{}, fmt.Errorf(
			"job %d current replan has %d exact same-session operation authorities",
			job.ID,
			currentReplans,
		)
	}
	if replan == nil && session.CurrentReplanOperationID != "" {
		return ObjectiveSessionContextAuthority{}, fmt.Errorf(
			"job %d session context has a current replan without generation authority",
			job.ID,
		)
	}
	if err := session.validate(replan); err != nil {
		return ObjectiveSessionContextAuthority{}, err
	}
	return session, nil
}

func (authority ObjectiveContinuityAuthority) ReplanContext() assemblyline.ObjectiveContext {
	return assemblyline.ObjectiveContext{ReplanAuthority: authority.Replan}
}

func (authority ObjectiveContinuityAuthority) ReplanRepresentedBySession() bool {
	return authority.Replan != nil && authority.Session != nil &&
		authority.Session.CurrentReplanOperationID != ""
}

func (authority ObjectiveContinuityAuthority) SessionContext() *ObjectiveSessionContextAuthority {
	if authority.Session == nil {
		return nil
	}
	cloned := *authority.Session
	cloned.Turns = append([]ObjectiveSessionTurnAuthority(nil), authority.Session.Turns...)
	return &cloned
}

func (authority ObjectiveContinuityAuthority) CodingFeedback() ([]string, error) {
	feedback := make([]string, 0)
	if authority.Session != nil {
		if err := authority.Session.validate(authority.Replan); err != nil {
			return nil, err
		}
		for _, turn := range authority.Session.Turns {
			if strings.TrimSpace(turn.ContextText) == "" {
				return nil, fmt.Errorf(
					"same-session operation %q has blank coding context",
					turn.OperationID,
				)
			}
			feedback = append(feedback, turn.ContextText)
		}
	}
	if authority.Replan != nil && !authority.ReplanRepresentedBySession() {
		feedback = append(feedback, "user redirect:\n"+authority.Replan.Feedback)
	}
	return feedback, nil
}

func (session ObjectiveSessionContextAuthority) validate(
	replan *assemblyline.ObjectiveReplanAuthority,
) error {
	if session.JobID < 1 || strings.TrimSpace(session.InitialInstruction) == "" {
		return fmt.Errorf("same-session context has invalid initial job authority")
	}
	contextBytes := 0
	currentReplans := 0
	for index, turn := range session.Turns {
		if _, err := ParseLifecycleOperationID(string(turn.OperationID)); err != nil {
			return err
		}
		if turn.Generation < 1 || turn.CreatedAt.IsZero() {
			return fmt.Errorf("same-session operation %q has invalid generation or time", turn.OperationID)
		}
		if index > 0 {
			left := session.Turns[index-1]
			if left.Generation > turn.Generation ||
				left.Generation == turn.Generation && left.CreatedAt.After(turn.CreatedAt) ||
				left.Generation == turn.Generation && left.CreatedAt.Equal(turn.CreatedAt) &&
					left.OperationID >= turn.OperationID {
				return fmt.Errorf("same-session operations are not strictly chronological")
			}
		}
		var expected string
		switch turn.Kind {
		case ObjectiveSessionFollowup:
			if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, turn.Text); err != nil {
				return err
			}
			expected = "user follow-up:\n" + turn.Text
		case ObjectiveSessionRedirect:
			if _, _, err := validateSessionReplanFeedback(turn.Text); err != nil {
				return err
			}
			expected = "user redirect:\n" + turn.Text
			if replan != nil && turn.Generation == replan.Generation && turn.Text == replan.Feedback {
				currentReplans++
				if session.CurrentReplanOperationID != turn.OperationID {
					return fmt.Errorf("current same-session redirect identity differs from replan authority")
				}
			}
		case ObjectiveSessionInterruption:
			if _, _, err := validateInterruptFeedback(turn.Text); err != nil {
				return err
			}
			expected = "user interruption:\n" + turn.Text
		case ObjectiveSessionCancellation:
			if _, err := validateCancelReason(turn.Text); err != nil {
				return err
			}
			expected = "user cancellation:\n" + turn.Text
		default:
			return fmt.Errorf("same-session operation %q has unregistered kind %q", turn.OperationID, turn.Kind)
		}
		if turn.ContextText != expected {
			return fmt.Errorf("same-session operation %q changed exact %s text", turn.OperationID, turn.Kind)
		}
		contextBytes += 2 + len(turn.ContextText)
		if contextBytes > MaxObjectiveSessionContextBytes {
			return fmt.Errorf(
				"job %d same-session context exceeds the explicit %d-byte bound",
				session.JobID,
				MaxObjectiveSessionContextBytes,
			)
		}
	}
	if replan != nil {
		if replan.JobID != session.JobID || currentReplans != 1 || session.CurrentReplanOperationID == "" {
			return fmt.Errorf("same-session current replan authority is incomplete")
		}
	} else if session.CurrentReplanOperationID != "" {
		return fmt.Errorf("same-session context has a current redirect without replan authority")
	}
	return nil
}
