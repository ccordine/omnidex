package queue

import (
	"context"
	"encoding/json"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Owner answers remain task-local and durable across a continuation replan.
// They are not execution state and cannot reopen an old native turn.
func (store *CodingAgentEvidenceStore) ReadTaskClarifications(ctx context.Context, taskID string) ([]codingagent.TaskClarification, error) {
	if err := store.repository.RequireActiveStepAttempt(ctx, store.authority); err != nil {
		return nil, err
	}
	rows, err := store.repository.pool.Query(ctx, `SELECT payload_json FROM evidence
	 WHERE job_id=$1 AND kind='coding_agent' AND COALESCE(payload_json->>'task_id','')=$2
	 AND (payload_json->>'generation')::bigint<=$3
	 AND payload_json->>'phase' IN ('execution_response','review_response','execution_native_input','execution_reply')
	 ORDER BY id`, store.authority.JobID, taskID, store.authority.Generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []codingagent.Entry{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var entry codingagent.Entry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return codingagent.ExtractTaskClarifications(entries, taskID), nil
}
