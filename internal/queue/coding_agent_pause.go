package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

func (repository *Repository) ReadCodingAgentPause(ctx context.Context, jobID, generation int64) (*model.CodingAgentPause, error) {
	pause := &model.CodingAgentPause{Schema: "omnidex-coding-agent-pause-v1"}
	var questions []byte
	err := repository.pool.QueryRow(ctx, `SELECT job.current_generation,step.id,step.current_attempt,question.payload_json->'questions'
	 FROM jobs job JOIN job_steps step ON step.job_id=job.id AND step.generation=job.current_generation
	 JOIN job_step_attempts attempt ON attempt.step_id=step.id AND attempt.attempt=step.current_attempt
	 JOIN LATERAL (SELECT payload_json FROM evidence WHERE job_id=job.id AND step_id=step.id AND kind='coding_agent' AND payload_json->>'generation'=job.current_generation::text AND payload_json->>'phase'='questions' ORDER BY id DESC LIMIT 1) question ON TRUE
	 WHERE job.id=$1 AND job.current_generation=$2 AND job.status='waiting_input'
	 AND job.metadata->'coding_execution'->>'mode'='agents' AND step.action='v3_coding'
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
