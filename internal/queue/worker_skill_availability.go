package queue

import (
	"context"
	"fmt"
)

// HasActiveWorkerSkills proves whether learned procedure retrieval has any
// durable work before provider identity is required. It also rejects an active
// version that lacks its one frozen embedding authority.
func (r *Repository) HasActiveWorkerSkills(ctx context.Context) (bool, error) {
	if r == nil || r.pool == nil {
		return false, fmt.Errorf("active worker skill availability requires PostgreSQL")
	}
	var invalid, exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM worker_skills AS skills
			LEFT JOIN worker_skill_embeddings AS embeddings
			  ON embeddings.skill_id=skills.skill_id
			 AND embeddings.skill_version=skills.version
			WHERE skills.status='active' AND skills.origin='learned'
			  AND skills.skill_kind='code_procedure'
			GROUP BY skills.skill_id,skills.version
			HAVING COUNT(embeddings.skill_id)<>1
		), EXISTS (
			SELECT 1 FROM worker_skills AS skills
			WHERE skills.status='active' AND skills.origin='learned'
			  AND skills.skill_kind='code_procedure'
		)
	`).Scan(&invalid, &exists)
	if err != nil {
		return false, fmt.Errorf("check active worker skill availability: %w", err)
	}
	if invalid {
		return false, fmt.Errorf(
			"active worker skill registry contains a version without exactly one frozen embedding identity",
		)
	}
	return exists, nil
}
