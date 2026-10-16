package queue

import "context"

// Accepted task identities filter the planner's prompt, not ledger authority.
// Replanning still carries only exact accepted contracts when the owner freezes.
func (r *Repository) CodingAcceptedTaskIDs(ctx context.Context, jobID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT task_item_id FROM coding_agent_tasks task
	 JOIN coding_ledgers ledger ON ledger.id=task.ledger_id WHERE ledger.job_id=$1
	 AND task.state='accepted' AND task.frozen_revision=(
	 SELECT max(frozen_revision) FROM coding_agent_tasks WHERE ledger_id=ledger.id)
	 ORDER BY task_item_id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
