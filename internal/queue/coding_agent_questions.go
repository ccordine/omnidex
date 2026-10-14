package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) PauseCodingAgentStep(ctx context.Context, authority model.StepAttemptAuthority, questions []string) error {
	if len(questions) == 0 || slices.ContainsFunc(questions, func(question string) bool { return strings.TrimSpace(question) == "" }) {
		return fmt.Errorf("agent pause requires concrete questions")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, "question pause requires a running job and step", nil)
	}
	if err := requireCodingAgentStepTx(ctx, tx, authority.JobID, authority.StepID, authority.Generation); err != nil {
		return err
	}
	question, err := latestCodingAgentQuestionTx(ctx, tx, authority.JobID, authority.StepID, authority.Generation)
	if err != nil {
		return err
	}
	if !slices.Equal(question.Questions, questions) {
		return fmt.Errorf("agent pause differs from its durable unanswered question")
	}
	if err := requireNoPendingChannelSessionMessagesTx(ctx, tx, authority.JobID, authority.Generation); err != nil {
		return err
	}
	if err := terminalizeStepAttemptTx(ctx, tx, authority, model.StepAttemptWaiting); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE job_steps SET status='waiting_input',worker_id=NULL,error=$2,updated_at=clock_timestamp() WHERE id=$1`, authority.StepID, strings.Join(questions, "\n")); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET status='waiting_input',error=NULL,updated_at=clock_timestamp() WHERE id=$1`, authority.JobID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func resumeCodingAgentQuestionTx(ctx context.Context, tx pgx.Tx, job model.Job, stepID int64, command SubmitJobFeedbackCommand) (model.Job, error) {
	if err := requireCodingAgentStepTx(ctx, tx, job.ID, stepID, job.CurrentGeneration); err != nil {
		return model.Job{}, err
	}
	var currentAttempt int64
	var status string
	if err := tx.QueryRow(ctx, `SELECT s.current_attempt,a.status FROM job_steps s JOIN job_step_attempts a ON a.job_id=s.job_id AND a.generation=s.generation AND a.step_id=s.id AND a.attempt=s.current_attempt WHERE s.id=$1 AND s.job_id=$2 AND s.generation=$3 AND s.status='waiting_input' FOR UPDATE OF s,a`, stepID, job.ID, job.CurrentGeneration).Scan(&currentAttempt, &status); err != nil {
		return model.Job{}, err
	}
	if status != string(model.StepAttemptWaiting) {
		return model.Job{}, fmt.Errorf("agent answer requires its terminal waiting attempt")
	}
	question, err := latestCodingAgentQuestionTx(ctx, tx, job.ID, stepID, job.CurrentGeneration)
	if err != nil {
		return model.Job{}, err
	}
	entry := codingagent.Entry{Generation: job.CurrentGeneration, Phase: "answers", OperationID: string(command.OperationID), QuestionID: question.ID, WaitingAttempt: currentAttempt, Answers: []string{command.Feedback}}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return model.Job{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,payload_json) VALUES($1,$2,'coding_agent',$3::jsonb)`, job.ID, stepID, string(encoded)); err != nil {
		return model.Job{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE job_steps SET status='running',worker_id=(SELECT worker_id FROM job_step_attempts WHERE step_id=$1 AND attempt=$2),error=NULL,updated_at=clock_timestamp() WHERE id=$1`, stepID, currentAttempt); err != nil {
		return model.Job{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET status='running',error=NULL,updated_at=clock_timestamp() WHERE id=$1`, job.ID); err != nil {
		return model.Job{}, err
	}
	return scanLockedJobTx(ctx, tx, job.ID)
}

// The latest question/answer transition is authoritative; earlier questions cannot
// be answered again after work resumes or a newer question replaces them.
func latestCodingAgentQuestionTx(ctx context.Context, tx pgx.Tx, jobID, stepID, generation int64) (codingagent.Entry, error) {
	var entry codingagent.Entry
	var raw []byte
	var id int64
	if err := tx.QueryRow(ctx, `SELECT id,payload_json FROM evidence WHERE job_id=$1 AND step_id=$2 AND kind='coding_agent' AND payload_json->>'generation'=$3 AND payload_json->>'phase' IN ('questions','answers') ORDER BY id DESC LIMIT 1`, jobID, stepID, fmt.Sprint(generation)).Scan(&id, &raw); err != nil {
		return entry, fmt.Errorf("agent answer requires a durable unanswered question: %w", err)
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return entry, err
	}
	entry.ID = id
	if entry.Generation != generation || entry.Phase != "questions" || len(entry.Questions) == 0 {
		return entry, fmt.Errorf("agent question was already answered or lacks exact generation")
	}
	return entry, nil
}

func requireCodingAgentStepTx(ctx context.Context, tx pgx.Tx, jobID, stepID, generation int64) error {
	var metadata []byte
	var action string
	if err := tx.QueryRow(ctx, `SELECT j.metadata,s.action FROM jobs j JOIN job_steps s ON s.job_id=j.id WHERE j.id=$1 AND s.id=$2 AND s.generation=$3`, jobID, stepID, generation).Scan(&metadata, &action); err != nil {
		return err
	}
	configuration, err := codingagent.ConfigurationFromMetadata(metadata)
	if err != nil {
		return err
	}
	if (action != "v3_coding_plan" && action != "v3_coding") || configuration.Mode != codingagent.Agents {
		return fmt.Errorf("coding agent evidence requires the selected Agents planning or execution step")
	}
	return nil
}

func requireCodingAgentAnswerReplayTx(ctx context.Context, tx pgx.Tx, jobID, stepID, generation int64, operationID model.LifecycleOperationID, answer string, expectedWaitingAttempt *int64) error {
	if err := requireCodingAgentStepTx(ctx, tx, jobID, stepID, generation); err != nil {
		return err
	}
	encodedAnswers, err := json.Marshal([]string{answer})
	if err != nil {
		return err
	}
	var valid bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence a JOIN evidence q ON q.id=(a.payload_json->>'question_id')::bigint AND q.job_id=a.job_id AND q.step_id=a.step_id AND q.kind='coding_agent' JOIN job_step_attempts waiting ON waiting.job_id=a.job_id AND waiting.step_id=a.step_id AND waiting.generation=$6 AND waiting.attempt=(a.payload_json->>'waiting_attempt')::bigint AND waiting.status='waiting_input' AND ($7::bigint IS NULL OR waiting.attempt=$7) WHERE a.job_id=$1 AND a.step_id=$2 AND a.kind='coding_agent' AND a.payload_json->>'generation'=$3 AND a.payload_json->>'phase'='answers' AND a.payload_json->>'operation_id'=$4 AND a.payload_json->'answers'=$5::jsonb AND q.payload_json->>'generation'=$3 AND q.payload_json->>'phase'='questions' AND q.id<a.id)`, jobID, stepID, fmt.Sprint(generation), operationID, string(encodedAnswers), generation, expectedWaitingAttempt).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return lifecycleReplayStateError(operationID, "durable scoped-agent question answer")
	}
	return requireLifecycleGenerationExistsTx(ctx, tx, jobID, generation)
}

func requireCodingAgentResumeClaimTx(ctx context.Context, tx pgx.Tx, jobID, stepID, generation, attempt int64) error {
	if err := requireCodingAgentStepTx(ctx, tx, jobID, stepID, generation); err != nil {
		return err
	}
	var valid bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT e.payload_json->>'phase'='answers' AND (e.payload_json->>'waiting_attempt')::bigint=a.attempt FROM evidence e JOIN job_step_attempts a ON a.job_id=e.job_id AND a.step_id=e.step_id AND a.generation=$4 AND a.attempt=$5 WHERE e.job_id=$1 AND e.step_id=$2 AND e.kind='coding_agent' AND e.payload_json->>'generation'=$3 AND e.payload_json->>'phase' IN ('questions','answers') ORDER BY e.id DESC LIMIT 1),false)`, jobID, stepID, fmt.Sprint(generation), generation, attempt).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("agent waiting attempt cannot resume without its durable answer")
	}
	return nil
}
