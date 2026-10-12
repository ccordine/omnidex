package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type CodingAgentEvidenceStore struct {
	repository *Repository
	authority  model.StepAttemptAuthority
}

func (r *Repository) CodingAgentEvidence(authority model.StepAttemptAuthority) *CodingAgentEvidenceStore {
	return &CodingAgentEvidenceStore{repository: r, authority: authority}
}

func (store *CodingAgentEvidenceStore) Append(ctx context.Context, entry codingagent.Entry) (int64, error) {
	if entry.ID != 0 || entry.Generation != store.authority.Generation || entry.Phase == "" {
		return 0, fmt.Errorf("agent evidence lacks its exact generation or phase")
	}
	encoded, err := json.Marshal(entry)
	if err != nil || len(encoded) > llm.MaxExactPreparedProviderResponseBytes {
		return 0, fmt.Errorf("agent evidence exceeds its typed transport boundary")
	}
	tx, err := store.repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, store.authority)
	if err != nil {
		return 0, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return 0, fmt.Errorf("agent evidence requires an active running coding step")
	}
	if err := requireCodingAgentStepTx(ctx, tx, store.authority.JobID, store.authority.StepID, store.authority.Generation); err != nil {
		return 0, err
	}
	if entry.Phase == "answers" || entry.OperationID != "" || entry.QuestionID != 0 || entry.WaitingAttempt != 0 {
		return 0, fmt.Errorf("agent answers require the exact user-feedback lifecycle operation")
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3::jsonb) RETURNING id`, store.authority.JobID, store.authority.StepID, string(encoded)).Scan(&id); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (store *CodingAgentEvidenceStore) Read(ctx context.Context) ([]codingagent.Entry, error) {
	if err := store.repository.RequireActiveStepAttempt(ctx, store.authority); err != nil {
		return nil, err
	}
	rows, err := store.repository.pool.Query(ctx, `SELECT id,payload_json FROM evidence WHERE job_id=$1 AND step_id=$2 AND kind='coding_agent' AND payload_json->>'generation'=$3 AND payload_json->>'phase' NOT IN ('native_event','call_opened','call_completed') ORDER BY id`, store.authority.JobID, store.authority.StepID, fmt.Sprint(store.authority.Generation))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []codingagent.Entry{}
	for rows.Next() {
		var id int64
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var entry codingagent.Entry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		if entry.Generation != store.authority.Generation {
			return nil, fmt.Errorf("agent evidence changed its generation")
		}
		entry.ID = id
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
