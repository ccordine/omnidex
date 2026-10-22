package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
	"github.com/jackc/pgx/v5"
)

var ErrUserIntentStale = errors.New("user intention refers to changed session or ledger state")

type InterpretUserIntent func(context.Context, sessionintent.Context) (sessionintent.Result, json.RawMessage, error)

// Interpretation and execution use different transactions. The stored result is
// untrusted intake, never a write permission or a claim of successful execution.
func (r *Repository) InterpretUserIntent(ctx context.Context, request sessionintent.Request, interpret InterpretUserIntent) (sessionintent.Result, error) {
	if _, err := model.ParseLifecycleOperationID(string(request.OperationID)); err != nil {
		return sessionintent.Result{}, err
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, request.Text); err != nil {
		return sessionintent.Result{}, err
	}
	if request.LedgerRevision < 0 || interpret == nil {
		return sessionintent.Result{}, fmt.Errorf("invalid user intent request")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return sessionintent.Result{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, request.OperationID); err != nil {
		return sessionintent.Result{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('user_intent'),hashtext($1))`, string(request.State.ChannelID)+":"+request.State.WorkspaceIdentity); err != nil {
		return sessionintent.Result{}, err
	}
	// Always check the current actor binding, including on an idempotent replay.
	if _, err := channelSessionStateTx(ctx, tx, request.State.ChannelID, request.State.WorkspaceIdentity); err != nil {
		return sessionintent.Result{}, err
	}
	var previous, raw []byte
	err = tx.QueryRow(ctx, `SELECT request_json,result_json FROM channel_user_intents WHERE operation_id=$1`, request.OperationID).Scan(&previous, &raw)
	if err == nil {
		expected, encodeErr := json.Marshal(request)
		if encodeErr != nil {
			return sessionintent.Result{}, encodeErr
		}
		if !sameCodingLedgerJSON(previous, expected) {
			return sessionintent.Result{}, ErrLifecycleOperationConflict
		}
		var result sessionintent.Result
		if err := json.Unmarshal(raw, &result); err != nil {
			return result, err
		}
		if err := tx.Commit(ctx); err != nil {
			return result, err
		}
		if result.Error != "" {
			return result, fmt.Errorf("persisted user interpretation failed: %s", result.Error)
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return sessionintent.Result{}, err
	}
	if err := rejectLifecycleUserIntentCollisionTx(ctx, tx, request.OperationID); err != nil {
		return sessionintent.Result{}, err
	}
	current, err := userIntentContextTx(ctx, tx, request.State.ChannelID, request.State.WorkspaceIdentity)
	if err != nil {
		return sessionintent.Result{}, err
	}
	if !current.State.Equal(request.State) || current.Revision != request.LedgerRevision && request.LedgerRevision > 0 {
		return sessionintent.Result{}, ErrUserIntentStale
	}
	displayed := userIntentDisplayedContext(current, request)
	if request.SelectedItem != "" {
		if !displayed.Planning {
			return sessionintent.Result{}, ErrUserIntentStale
		}
		found := false
		for _, item := range current.Items {
			if item.ID == request.SelectedItem {
				displayed.Facts = append(displayed.Facts, "The owner selected this displayed plan item: "+item.Text)
				found = true
			}
		}
		if !found {
			return sessionintent.Result{}, ErrUserIntentStale
		}
	}
	if err := request.ValidateRouting(displayed); err != nil {
		return sessionintent.Result{}, err
	}
	result, evidence, interpretationErr := interpret(ctx, displayed)
	if interpretationErr != nil {
		if len(evidence) == 0 {
			return sessionintent.Result{}, interpretationErr
		}
		result = sessionintent.Result{OperationID: request.OperationID, Patch: []userintent.Patch{}, Error: interpretationErr.Error()}
	}
	if result.OperationID != request.OperationID {
		return sessionintent.Result{}, fmt.Errorf("interpreted user message identity changed")
	}
	result.RequiresApproval = false
	result.Pitch = ""
	if result.Error == "" && result.Clarification == "" {
		result.Patch, err = sessionintent.BindContent(request, displayed, result.Patch)
		if err != nil {
			return sessionintent.Result{}, err
		}
		if err := sessionintent.ValidateRoutedPatch(request, displayed, result.Patch); err != nil {
			return sessionintent.Result{}, err
		}
		after, err := sessionintent.Apply(displayed, result.Patch)
		if err != nil {
			return sessionintent.Result{}, err
		}
		result.RequiresApproval = sessionintent.NeedsApproval(displayed, after)
		if result.RequiresApproval {
			result.Pitch = sessionintent.ConfirmationPitch(displayed, after)
		} else {
			result.Pitch = ""
		}
	} else if len(result.Patch) != 0 {
		return sessionintent.Result{}, fmt.Errorf("ambiguous user intent must have no state changes")
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return result, err
	}
	contextJSON, err := json.Marshal(current)
	if err != nil {
		return result, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	if len(evidence) == 0 || !json.Valid(evidence) {
		return result, fmt.Errorf("user intent model evidence is absent or exceeds 4 MiB")
	}
	if result.RequiresApproval {
		_, err := tx.Exec(ctx, `UPDATE channel_user_intents SET receipt_json=jsonb_build_object('operation_id',operation_id,'patch',result_json->'patch','summary','Superseded by a later proposed action.') WHERE channel_id=$1 AND workspace_identity=$2 AND receipt_json IS NULL AND result_json->>'requires_approval'='true'`, request.State.ChannelID, request.State.WorkspaceIdentity)
		if err != nil {
			return result, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO channel_user_intents(operation_id,channel_id,workspace_identity,request_json,context_json,result_json,evidence_json)
	 VALUES($1,$2,$3,$4,$5,$6,$7)`, request.OperationID, request.State.ChannelID, request.State.WorkspaceIdentity, requestJSON, contextJSON, resultJSON, evidence)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	if interpretationErr != nil {
		return result, interpretationErr
	}
	return result, nil
}

func userIntentContextTx(ctx context.Context, tx pgx.Tx, channelID model.ChannelID, identity string) (sessionintent.Context, error) {
	state, err := channelSessionStateTx(ctx, tx, channelID, identity)
	if err != nil {
		return sessionintent.Context{}, err
	}
	c := sessionintent.Context{State: state, Items: []sessionintent.Item{}}
	if err := userIntentConversationTx(ctx, tx, &c); err != nil {
		return c, err
	}
	if state.LatestJob == nil || state.LatestJob.Status == model.JobStatusCompleted || state.LatestJob.Status == model.JobStatusCanceled {
		return c, nil
	}
	details, err := currentJobDetailsTx(ctx, tx, state.LatestJob.ID)
	if err != nil {
		return c, err
	}
	if _, _, recoveryErr := jobRecoveryStepTx(ctx, tx, details.Job); recoveryErr == nil {
		c.Recoverable = true
	} else if !errors.Is(recoveryErr, pgx.ErrNoRows) {
		return c, recoveryErr
	}
	if details.Job.Status == model.JobStatusFailed && !c.Recoverable {
		return c, nil
	}
	c.JobID, c.Generation, c.JobStatus, c.InputID = details.Job.ID, details.Job.CurrentGeneration, details.Job.Status, details.ExecutionInputID
	for _, step := range details.Steps {
		if step.Action == "v3_coding_plan" && step.Generation == c.Generation && step.SupersededAtGeneration == nil {
			c.Planning = step.Status == model.StepStatusWaiting
		}
	}
	var ledgerID, revision int64
	var planState string
	err = tx.QueryRow(ctx, `SELECT id,current_revision,state FROM coding_ledgers WHERE job_id=$1`, c.JobID).Scan(&ledgerID, &revision, &planState)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return c, err
	}
	if err == nil {
		c.LedgerID, c.Revision, c.PlanState = ledgerID, revision, planState
		ledger, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, revision, CodingLedgerReadOptions{Limit: -1})
		if err != nil {
			return c, err
		}
		for _, task := range ledger.Ledger.Tasks {
			if task.Disposition == model.LedgerDispositionActive {
				c.Items = append(c.Items, sessionintent.Item{ID: task.ID, Text: task.Goal, ReviewState: task.ReviewState, Disposition: task.Disposition})
			}
		}
	}
	if c.InputID != nil {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT payload_json FROM evidence WHERE id=$1 AND job_id=$2 AND kind='coding_agent'`, *c.InputID, c.JobID).Scan(&raw); err != nil {
			return c, err
		}
		var entry codingagent.Entry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return c, err
		}
		if entry.NativeInput != nil {
			c.Prompt = entry.NativeInput.Prompt
			for _, choice := range entry.NativeInput.Choices {
				c.NativeChoices = append(c.NativeChoices, choice.Label)
			}
		}
	}

	c.Facts = append(c.Facts, fmt.Sprintf("Current authoritative job state: %s. Current plan state: %s. These persisted facts take precedence over conversational claims of approval or completion.", c.JobStatus, c.PlanState))
	return c, nil
}
