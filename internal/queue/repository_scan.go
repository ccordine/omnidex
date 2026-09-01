package queue

import (
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func scanJob(row pgx.Row) (model.Job, error) {
	var job model.Job
	var result, errText *string
	if err := row.Scan(
		&job.ID,
		&job.Instruction,
		&job.Pipeline,
		&job.Status,
		&result,
		&errText,
		&job.Metadata,
		&job.CurrentGeneration,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.CompletedAt,
	); err != nil {
		return model.Job{}, err
	}
	job.Result = stringOrEmpty(result)
	job.Error = stringOrEmpty(errText)
	if len(job.Metadata) == 0 {
		job.Metadata = []byte(`{}`)
	}
	return job, nil
}

func scanStep(row pgx.Row) (model.Step, error) {
	var step model.Step
	var workerID, output, errText *string
	if err := row.Scan(
		&step.ID,
		&step.JobID,
		&step.Action,
		&step.SortIndex,
		&step.Status,
		&step.Generation,
		&step.SupersededAtGeneration,
		&workerID,
		&output,
		&errText,
		&step.StartedAt,
		&step.FinishedAt,
		&step.CreatedAt,
		&step.UpdatedAt,
	); err != nil {
		return model.Step{}, err
	}
	step.WorkerID = stringOrEmpty(workerID)
	step.Output = stringOrEmpty(output)
	step.Error = stringOrEmpty(errText)
	return step, nil
}

func scanClaim(row pgx.Row) (model.Step, model.Job, error) {
	var step model.Step
	var job model.Job
	var stepWorker, stepOutput, stepError *string
	var jobResult, jobError *string
	if err := row.Scan(
		&step.ID,
		&step.JobID,
		&step.Action,
		&step.SortIndex,
		&step.Status,
		&step.Generation,
		&step.SupersededAtGeneration,
		&stepWorker,
		&stepOutput,
		&stepError,
		&step.StartedAt,
		&step.FinishedAt,
		&step.CreatedAt,
		&step.UpdatedAt,
		&job.ID,
		&job.Instruction,
		&job.Pipeline,
		&job.Status,
		&jobResult,
		&jobError,
		&job.Metadata,
		&job.CurrentGeneration,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.CompletedAt,
	); err != nil {
		return model.Step{}, model.Job{}, err
	}
	step.WorkerID = stringOrEmpty(stepWorker)
	step.Output = stringOrEmpty(stepOutput)
	step.Error = stringOrEmpty(stepError)
	job.Result = stringOrEmpty(jobResult)
	job.Error = stringOrEmpty(jobError)
	if len(job.Metadata) == 0 {
		job.Metadata = []byte(`{}`)
	}
	return step, job, nil
}
