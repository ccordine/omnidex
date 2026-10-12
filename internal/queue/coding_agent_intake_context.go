package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (repository *Repository) CodingAgentIntakeContext(ctx context.Context, authority model.StepAttemptAuthority) (*codingagent.IntakeContext, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return nil, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return nil, staleStepAttemptError(authority, "intake projection requires current running execution", nil)
	}
	if err := requireCodingAgentStepTx(ctx, tx, authority.JobID, authority.StepID, authority.Generation); err != nil {
		return nil, err
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT receipt.payload_json->'intake_context' FROM coding_plans plan JOIN evidence receipt ON receipt.job_id=plan.job_id AND receipt.step_id=plan.plan_step_id AND receipt.kind='coding_agent' AND receipt.payload_json->>'phase'='requirements_frozen' AND receipt.payload_json->>'generation'=plan.generation::text WHERE plan.job_id=$1 AND plan.generation=$2 AND plan.state='frozen'`, authority.JobID, authority.Generation).Scan(&raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var retained codingagent.IntakeContext
	if err := decoder.Decode(&retained); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("intake context has trailing JSON")
	}
	if err := retained.Validate(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &retained, nil
}
