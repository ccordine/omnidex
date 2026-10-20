package queue

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/jackc/pgx/v5"
)

func userIntentConversationTx(ctx context.Context, tx pgx.Tx, c *sessionintent.Context) error {
	// Exactly two recent exchanges, plus explicitly scoped persisted system facts.
	// There is no full-history loading or keyword-based relevance scan.
	rows, err := tx.Query(ctx, `SELECT role,content FROM (SELECT *,row_number() OVER (PARTITION BY role ORDER BY created_at DESC,tie DESC) AS role_rank FROM (
  SELECT created_at,id::text AS tie,role,content FROM ai_channel_messages message WHERE channel_id=$1 AND role IN ('user','assistant') AND NOT EXISTS(SELECT 1 FROM channel_user_intents intent WHERE intent.receipt_json->>'message_id'=message.id::text)
  UNION ALL SELECT created_at,operation_id||'_0','user',request_json->>'text' FROM channel_user_intents WHERE channel_id=$1 AND workspace_identity=$2
  UNION ALL SELECT created_at,operation_id||'_1','assistant',COALESCE(receipt_json->>'summary',NULLIF(result_json->>'pitch',''),result_json->>'clarification') FROM channel_user_intents WHERE channel_id=$1 AND workspace_identity=$2
 ) recent WHERE content IS NOT NULL AND content<>'') ranked WHERE role_rank<=2 ORDER BY created_at DESC,tie DESC LIMIT 4`, c.State.ChannelID, c.State.WorkspaceIdentity)
	if err != nil {
		return err
	}
	c.Recent = []sessionintent.Message{}
	for rows.Next() {
		var message sessionintent.Message
		if err := rows.Scan(&message.Role, &message.Text); err != nil {
			rows.Close()
			return err
		}
		c.Recent = append(c.Recent, message)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for i, j := 0, len(c.Recent)-1; i < j; i, j = i+1, j-1 {
		c.Recent[i], c.Recent[j] = c.Recent[j], c.Recent[i]
	}
	c.Pending, err = readPendingUserProposalTx(ctx, tx, c.State)
	return err
}

func readPendingUserProposalTx(ctx context.Context, tx pgx.Tx, state model.ChannelSessionState) (*sessionintent.Proposal, error) {
	var raw, bound []byte
	err := tx.QueryRow(ctx, `SELECT result_json,context_json FROM channel_user_intents WHERE channel_id=$1 AND workspace_identity=$2 AND receipt_json IS NULL AND result_json->>'requires_approval'='true' AND (context_json->>'revision')::bigint=COALESCE((SELECT current_revision FROM coding_ledgers WHERE job_id=(context_json->>'job_id')::bigint),0) ORDER BY created_at DESC,operation_id DESC LIMIT 1`, state.ChannelID, state.WorkspaceIdentity).Scan(&raw, &bound)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result sessionintent.Result
	var current sessionintent.Context
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(bound, &current); err != nil {
		return nil, err
	}
	if !current.State.Equal(state) {
		return nil, nil
	}
	return &sessionintent.Proposal{ID: result.OperationID, Pitch: result.Pitch}, nil
}

// Persisted planning state is not evidence that the caller displayed a review.
// Keep the authoritative context intact for execution-time stale checks.
func userIntentDisplayedContext(current sessionintent.Context, request sessionintent.Request) sessionintent.Context {
	if request.LedgerRevision == 0 {
		current.Planning = false
		current.Items = []sessionintent.Item{}
	}
	return current
}
