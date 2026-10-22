package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/jackc/pgx/v5"
)

type ResearchAnswer func(context.Context, sessionintent.ResearchContext) (string, json.RawMessage, error)

func (r *Repository) ResearchHistory(ctx context.Context, channel model.ChannelID, identity string) ([]sessionintent.ResearchTurn, error) {
	return r.ResearchConversationHistory(ctx, channel, identity, "")
}

func (r *Repository) ResearchConversationHistory(ctx context.Context, channel model.ChannelID, identity string, conversation model.LifecycleOperationID) ([]sessionintent.ResearchTurn, error) {
	turns := []sessionintent.ResearchTurn{}
	for offset := 0; ; offset += sessionintent.ResearchHistoryPageSize {
		page, err := r.ResearchConversationHistoryPage(ctx, channel, identity, conversation, offset)
		if err != nil {
			return nil, err
		}
		turns = append(turns, page...)
		if len(page) < sessionintent.ResearchHistoryPageSize {
			return turns, nil
		}
	}
}

func (r *Repository) ResearchConversationHistoryPage(ctx context.Context, channel model.ChannelID, identity string, conversation model.LifecycleOperationID, offset int) ([]sessionintent.ResearchTurn, error) {
	if offset < 0 {
		return nil, fmt.Errorf("research history offset must be nonnegative")
	}
	if _, err := r.ChannelSessionState(ctx, channel, identity); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT result_json FROM channel_research_turns WHERE channel_id=$1 AND workspace_identity=$2 AND COALESCE(result_json->>'conversation_id','')=$3 ORDER BY created_at,operation_id LIMIT $4 OFFSET $5`, channel, identity, conversation, sessionintent.ResearchHistoryPageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	turns := []sessionintent.ResearchTurn{}
	for rows.Next() {
		var raw []byte
		var turn sessionintent.ResearchTurn
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &turn); err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, rows.Err()
}

// Research serializes only its own conversation. It neither locks nor updates
// the coding job, ledger, native input, or pending approvals.
func (r *Repository) Research(ctx context.Context, channel model.ChannelID, request sessionintent.ResearchRequest, answer ResearchAnswer) (sessionintent.ResearchTurn, error) {
	result := sessionintent.ResearchTurn{OperationID: request.OperationID, Question: request.Text, ConversationID: request.ConversationID}
	if _, err := model.ParseLifecycleOperationID(string(request.OperationID)); err != nil {
		return result, err
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, request.Text); err != nil {
		return result, err
	}
	if answer == nil {
		return result, fmt.Errorf("research provider is absent")
	}
	// Keep the outcome transaction alive long enough to retain cancellation.
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
	defer cancel()
	tx, err := r.pool.Begin(persistCtx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(persistCtx)
	if err := lockLifecycleOperationIdentityTx(persistCtx, tx, request.OperationID); err != nil {
		return result, err
	}
	if _, err := tx.Exec(persistCtx, `SELECT pg_advisory_xact_lock(hashtext('research'),hashtext($1))`, string(channel)+":"+request.WorkspaceIdentity+":"+string(request.ConversationID)); err != nil {
		return result, err
	}
	state, err := channelSessionStateTx(persistCtx, tx, channel, request.WorkspaceIdentity)
	if err != nil {
		return result, err
	}
	requestJSON, err := json.Marshal(struct {
		sessionintent.ResearchRequest
		ChannelID model.ChannelID `json:"channel_id"`
	}{request, channel})
	if err != nil {
		return result, err
	}
	var oldRequest, raw []byte
	err = tx.QueryRow(persistCtx, `SELECT request_json,result_json FROM channel_research_turns WHERE operation_id=$1 AND channel_id=$2`, request.OperationID, channel).Scan(&oldRequest, &raw)
	if err == nil {
		if !sameCodingLedgerJSON(oldRequest, requestJSON) {
			return result, ErrLifecycleOperationConflict
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return result, err
		}
		return result, tx.Commit(persistCtx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	// Reserve the global operation identity, as with other typed session turns.
	if _, err := reserveLifecycleOperationIdentityTx(persistCtx, tx, request.OperationID, LifecycleResearch, requestJSON); err != nil {
		return result, err
	}
	seed, err := researchForkTx(persistCtx, tx, channel, request)
	if err != nil {
		return result, err
	}
	result.Fork = seed
	history, err := r.ResearchConversationHistory(persistCtx, channel, request.WorkspaceIdentity, request.ConversationID)
	if err != nil {
		return result, err
	}
	if len(history) > 2 {
		history = history[len(history)-2:]
	}
	current := sessionintent.ResearchContext{State: state, Recent: history, Fork: seed}
	text, evidence, researchErr := answer(ctx, current)
	if researchErr != nil {
		result.Error = researchErr.Error()
	} else {
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleAssistant, text); err != nil {
			return result, err
		}
		result.Answer = text
	}
	if !json.Valid(evidence) {
		return result, fmt.Errorf("research evidence is absent or exceeds 4 MiB")
	}
	contextJSON, err := json.Marshal(current)
	if err != nil {
		return result, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	_, err = tx.Exec(persistCtx, `INSERT INTO channel_research_turns(operation_id,channel_id,workspace_identity,request_json,context_json,result_json,evidence_json) VALUES($1,$2,$3,$4,$5,$6,$7)`, request.OperationID, channel, request.WorkspaceIdentity, requestJSON, contextJSON, resultJSON, evidence)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(persistCtx)
}
