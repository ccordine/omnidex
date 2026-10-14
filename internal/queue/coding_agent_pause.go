package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type codingAgentPauseQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (repository *Repository) ReadCodingAgentPause(ctx context.Context, jobID, generation int64) (*model.CodingAgentPause, error) {
	return readCodingAgentPause(ctx, repository.pool, jobID, generation)
}

func readCurrentCodingAgentPauseTx(ctx context.Context, tx pgx.Tx, job model.Job, steps []model.Step) (*model.CodingAgentPause, error) {
	configuration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil {
		return nil, err
	}
	if job.Status != model.JobStatusWaiting || configuration.Mode != codingagent.Agents {
		return nil, nil
	}
	if !slices.ContainsFunc(steps, func(step model.Step) bool {
		return step.JobID == job.ID && step.Generation == job.CurrentGeneration &&
			step.SupersededAtGeneration == nil &&
			(step.Action == "v3_coding_plan" || step.Action == "v3_coding") && step.Status == model.StepStatusWaiting
	}) {
		return nil, nil
	}
	var purpose string
	if err := tx.QueryRow(ctx, `SELECT purpose FROM job_generations WHERE job_id=$1 AND generation=$2`, job.ID, job.CurrentGeneration).Scan(&purpose); err != nil {
		return nil, err
	}
	switch purpose {
	case jobGenerationPurposeInterrupt:
		return nil, nil
	case jobGenerationPurposeInitial, jobGenerationPurposeReplan:
		pause, err := readCodingAgentPause(ctx, tx, job.ID, job.CurrentGeneration)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return pause, err
	default:
		return nil, fmt.Errorf("%w: job %d generation %d has unregistered purpose %q", ErrInvalidJobGeneration, job.ID, job.CurrentGeneration, purpose)
	}
}

func readCodingAgentPause(ctx context.Context, querier codingAgentPauseQuerier, jobID, generation int64) (*model.CodingAgentPause, error) {
	pause := &model.CodingAgentPause{Schema: "omnidex-coding-agent-pause-v1"}
	var questions []byte
	err := querier.QueryRow(ctx, `SELECT job.current_generation,step.id,step.current_attempt,question.payload_json->'questions'
	 FROM jobs job JOIN job_steps step ON step.job_id=job.id AND step.generation=job.current_generation
	 JOIN job_step_attempts attempt ON attempt.step_id=step.id AND attempt.attempt=step.current_attempt
	 JOIN LATERAL (SELECT payload_json FROM evidence WHERE job_id=job.id AND step_id=step.id AND kind='coding_agent' AND payload_json->>'generation'=job.current_generation::text AND payload_json->>'phase'='questions' ORDER BY id DESC LIMIT 1) question ON TRUE
	 WHERE job.id=$1 AND job.current_generation=$2 AND job.status='waiting_input'
	 AND job.metadata->'coding_execution'->>'mode'='agents' AND step.action IN ('v3_coding_plan','v3_coding')
	 AND step.status='waiting_input' AND step.superseded_at_generation IS NULL AND attempt.status='waiting_input'`, jobID, generation).Scan(&pause.Generation, &pause.StepID, &pause.WaitingAttempt, &questions)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(questions, &pause.Questions); err != nil {
		return nil, err
	}
	if len(pause.Questions) == 0 || pause.WaitingAttempt < 1 {
		return nil, fmt.Errorf("agent pause lacks its concrete durable question")
	}
	return pause, nil
}
