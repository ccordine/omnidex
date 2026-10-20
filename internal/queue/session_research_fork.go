package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/jackc/pgx/v5"
)

// Resolve only the owner's explicit source selection, scoped to this session.
func researchForkTx(ctx context.Context, tx pgx.Tx, channel model.ChannelID, request sessionintent.ResearchRequest) (*sessionintent.ResearchSeed, error) {
	if request.ConversationID == "" {
		if request.Fork != nil {
			return nil, fmt.Errorf("fork requires a conversation identity")
		}
		return nil, nil
	}
	if _, err := model.ParseLifecycleOperationID(string(request.ConversationID)); err != nil {
		return nil, err
	}
	if request.Fork == nil {
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT result_json FROM channel_research_turns WHERE operation_id=$1 AND channel_id=$2 AND workspace_identity=$3 AND result_json->>'conversation_id'=$1`, request.ConversationID, channel, request.WorkspaceIdentity).Scan(&raw)
		if err != nil {
			return nil, fmt.Errorf("research conversation not found: %w", err)
		}
		var first sessionintent.ResearchTurn
		if err := json.Unmarshal(raw, &first); err != nil {
			return nil, err
		}
		if first.Fork == nil {
			return nil, fmt.Errorf("research conversation lacks its source")
		}
		return first.Fork, nil
	}
	seed := request.Fork
	if request.ConversationID != request.OperationID || seed.SourceChannel != channel || len(seed.Messages) != 0 || len(seed.Ledger) != 0 {
		return nil, fmt.Errorf("fork must select this session's source references, without supplied source content")
	}
	if len(seed.MessageIDs) > 20 || (len(seed.MessageIDs) == 0) == (seed.LedgerID == 0) || (seed.LedgerID == 0) != (seed.LedgerRevision == 0) {
		return nil, fmt.Errorf("select one to twenty messages or one exact ledger revision")
	}
	resolved := &sessionintent.ResearchSeed{SourceChannel: channel, MessageIDs: seed.MessageIDs, LedgerID: seed.LedgerID, LedgerRevision: seed.LedgerRevision}
	seen := map[int64]bool{}
	for _, id := range seed.MessageIDs {
		if id <= 0 || seen[id] {
			return nil, fmt.Errorf("message selection contains invalid or duplicate identity")
		}
		seen[id] = true
		var m model.ChannelMessage
		if err := tx.QueryRow(ctx, `SELECT id,channel_id,role,content,created_at FROM ai_channel_messages WHERE channel_id=$1 AND id=$2`, channel, id).Scan(&m.ID, &m.ChannelID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		resolved.Messages = append(resolved.Messages, sessionintent.ResearchMessage{ID: m.ID, Role: string(m.Role), Text: m.Content})
	}
	if seed.LedgerID != 0 {
		var belongs bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coding_ledgers l JOIN jobs j ON j.id=l.job_id WHERE l.id=$1 AND j.metadata->>'channel_id'=$2)`, seed.LedgerID, channel).Scan(&belongs); err != nil {
			return nil, err
		}
		if !belongs {
			return nil, fmt.Errorf("ledger does not belong to source session")
		}
		ledger, err := readCodingLedgerRevisionTx(ctx, tx, seed.LedgerID, seed.LedgerRevision, CodingLedgerReadOptions{Limit: 101})
		if err != nil {
			return nil, err
		}
		if len(ledger.Ledger.Tasks) > 100 {
			return nil, fmt.Errorf("selected ledger exceeds research context budget")
		}
		resolved.Ledger, err = json.Marshal(ledger.Ledger)
		if err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(resolved)
	if err != nil {
		return nil, err
	}
	if len(raw) > 64<<10 {
		return nil, fmt.Errorf("fork source exceeds 64 KiB")
	}
	return resolved, nil
}
