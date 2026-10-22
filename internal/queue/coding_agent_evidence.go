package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/codingagent"
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
	if err != nil {
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
	if entry.Phase == "questions" || entry.Phase == "answers" {
		return 0, fmt.Errorf("coding agents cannot create owner-input state")
	}
	if entry.PlanningProposal != nil {
		if entry.Phase != "planning_response" || len(entry.Notes) != 3 {
			return 0, fmt.Errorf("published proposal lacks its operation provenance")
		}
		var ledgerID, current int64
		if err := tx.QueryRow(ctx, `SELECT id,current_revision FROM coding_ledgers WHERE job_id=$1 FOR UPDATE`, store.authority.JobID).Scan(&ledgerID, &current); err != nil {
			return 0, err
		}
		projection, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, current, CodingLedgerReadOptions{Limit: -1})
		if err != nil {
			return 0, err
		}
		expected, actual := *entry.PlanningProposal, projection.Ledger
		sortCodingLedgerProjection(&expected)
		sortCodingLedgerProjection(&actual)
		if !reflect.DeepEqual(expected, actual) {
			return 0, fmt.Errorf("%w: proposal changed before publication", ErrCodingLedgerRevision)
		}
		var prior int64
		err = tx.QueryRow(ctx, `SELECT id FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'phase'='planning_response' AND payload_json->'notes'->>0=$2 ORDER BY id LIMIT 1`, store.authority.JobID, entry.Notes[0]).Scan(&prior)
		if err == nil {
			return prior, tx.Commit(ctx)
		}
		if err != pgx.ErrNoRows {
			return 0, err
		}
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3::jsonb) RETURNING id`, store.authority.JobID, store.authority.StepID, string(encoded)).Scan(&id); err != nil {
		return 0, err
	}
	message := ""
	if entry.Phase == "planning_response" && len(entry.Notes) == 3 {
		message = entry.Notes[2]
	}
	if (entry.Phase == "execution_response" || entry.Phase == "review_response" || entry.Phase == "task_incomplete" || entry.Phase == "planning_incomplete") && len(entry.Notes) == 1 {
		message = entry.Notes[0]
	}
	if message != "" {
		if err := appendCodingAssistantMessagesTx(ctx, tx, store.authority.JobID, message); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit(ctx)
}

func (store *CodingAgentEvidenceStore) Read(ctx context.Context) ([]codingagent.Entry, error) {
	if err := store.repository.RequireActiveStepAttempt(ctx, store.authority); err != nil {
		return nil, err
	}
	rows, err := store.repository.pool.Query(ctx, `
		SELECT evidence.id,evidence.payload_json
		FROM evidence
		JOIN job_steps AS source_step ON source_step.id=evidence.step_id AND source_step.job_id=evidence.job_id
		JOIN job_steps AS current_step ON current_step.id=$2 AND current_step.job_id=evidence.job_id
		WHERE evidence.job_id=$1
		  AND (source_step.action=current_step.action OR
		       (current_step.action='v3_coding_plan' AND evidence.payload_json->>'phase'='task_incomplete'))
		  AND evidence.kind='coding_agent'
		  AND (
		    (source_step.generation=$3 AND evidence.payload_json->>'generation'=$3::text
		     AND (evidence.payload_json->>'phase' NOT IN ('native_event','call_opened','call_completed')
		          OR (evidence.payload_json->>'phase'='call_completed' AND evidence.payload_json->>'role' IN ('execute','review'))))
		    OR (current_step.action='v3_coding_plan' AND source_step.generation<=$3
		        AND evidence.payload_json->>'phase' IN ('planning_response','planning_draft','task_ordering','task_incomplete','planning_incomplete'))
		  )
		ORDER BY evidence.id
	`, store.authority.JobID, store.authority.StepID, store.authority.Generation)
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
		if entry.Generation != store.authority.Generation &&
			!((entry.Phase == "planning_response" || entry.Phase == "planning_draft" || entry.Phase == "task_ordering" || entry.Phase == "task_incomplete" || entry.Phase == "planning_incomplete") && entry.Generation > 0 && entry.Generation < store.authority.Generation) {
			return nil, fmt.Errorf("agent evidence changed its generation")
		}
		entry.ID = id
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
