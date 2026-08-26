package queue

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestGeneratedWorkloadDeploymentBlocksEveryAuthorityChangingLifecyclePath(t *testing.T) {
	for _, state := range []GeneratedWorkloadDeploymentState{
		GeneratedWorkloadDeploymentPrepared,
		GeneratedWorkloadDeploymentApplying,
		GeneratedWorkloadDeploymentIndeterminate,
	} {
		state := state
		t.Run(string(state), func(t *testing.T) {
			fixture := newGeneratedDeploymentDatabaseFixture(t, "authority-change-"+string(state))
			record := prepareGeneratedDeploymentInState(t, fixture, state)
			before := generatedDeploymentAuthorityState(t, fixture)

			operations := []struct {
				name string
				run  func() error
			}{
				{name: "fail step", run: func() error {
					return fixture.repository.FailStep(fixture.ctx, FailStepCommand{
						OperationID: testLifecycleOperationID(t, "deployment-fail-"+string(state), fixture.authority.StepID),
						Authority:   fixture.authority, StepID: fixture.authority.StepID,
						Error: "runtime returned after durable deployment authority was established",
					})
				}},
				{name: "complete step", run: func() error {
					return fixture.repository.CompleteStep(fixture.ctx, CompleteStepCommand{
						OperationID: testLifecycleOperationID(t, "deployment-complete-"+string(state), fixture.authority.StepID),
						Authority:   fixture.authority, StepID: fixture.authority.StepID,
						Output: "must not complete while deployment recovery is unresolved",
					})
				}},
				{name: "pause step", run: func() error {
					return fixture.repository.PauseStepForInput(
						fixture.ctx, fixture.authority, "must remain running",
						"must not wait while deployment recovery is unresolved", nil,
					)
				}},
				{name: "replan job", run: func() error {
					_, err := fixture.repository.ReplanJob(fixture.ctx, testReplanCommand(
						t, fixture.jobID, "deployment-replan-"+string(state),
						"must recover the durable deployment before replanning",
					))
					return err
				}},
				{name: "cancel job", run: func() error {
					_, err := fixture.repository.CancelJob(fixture.ctx, testCancelCommand(
						t, fixture.jobID, "deployment-cancel-"+string(state),
						"must recover the durable deployment before cancellation",
					))
					return err
				}},
			}
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					err := operation.run()
					assertGeneratedDeploymentAuthorityChangeRejected(t, err, fixture, record, state)
					after := generatedDeploymentAuthorityState(t, fixture)
					if after != before {
						t.Fatalf("rejected %s changed job/step/attempt authority: before=%+v after=%+v", operation.name, before, after)
					}
				})
			}
		})
	}
}

func TestUnresolvedGeneratedWorkloadDeploymentFindsForeignProjectCandidate(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixture(t, "foreign-candidate")
	record := prepareGeneratedDeploymentInState(t, fixture, GeneratedWorkloadDeploymentApplying)
	project, err := fixture.repository.GetProject(fixture.ctx, fixture.projectID)
	if err != nil {
		t.Fatal(err)
	}
	metadata := fmt.Sprintf(`{"project_id":%d,"client_cwd":%q}`, fixture.projectID, project.Location)
	job, err := fixture.repository.EnqueueJob(
		fixture.ctx, "foreign-candidate-observer", model.PipelineCoding, []byte(metadata),
	)
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := fixture.repository.UnresolvedGeneratedWorkloadDeployment(
		fixture.ctx, job.ID, job.CurrentGeneration,
	)
	if err != nil {
		t.Fatal(err)
	}
	if blocker == nil || blocker.OperationID != record.OperationID ||
		blocker.JobID != fixture.jobID || blocker.Generation != fixture.authority.Generation ||
		blocker.ProjectID != fixture.projectID || blocker.State != GeneratedWorkloadDeploymentApplying ||
		!blocker.Candidate {
		t.Fatalf("foreign candidate blocker=%+v", blocker)
	}
	current, err := fixture.repository.UnresolvedGeneratedWorkloadDeployment(
		fixture.ctx, fixture.jobID, fixture.authority.Generation,
	)
	if err != nil || current != nil {
		t.Fatalf("current generation was reported as historical blocker=%+v err=%v", current, err)
	}
}

func TestUnresolvedGeneratedWorkloadDeploymentFindsHistoricalGeneration(t *testing.T) {
	fixture := newGeneratedDeploymentDatabaseFixture(t, "historical-generation")
	record := prepareGeneratedDeploymentInState(t, fixture, GeneratedWorkloadDeploymentPrepared)
	blocker, err := fixture.repository.UnresolvedGeneratedWorkloadDeployment(
		fixture.ctx, fixture.jobID, fixture.authority.Generation+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if blocker == nil || blocker.OperationID != record.OperationID ||
		blocker.JobID != fixture.jobID || blocker.Generation != fixture.authority.Generation ||
		blocker.ProjectID != fixture.projectID || blocker.State != GeneratedWorkloadDeploymentPrepared ||
		blocker.Candidate {
		t.Fatalf("historical deployment blocker=%+v", blocker)
	}
}

func prepareGeneratedDeploymentInState(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
	state GeneratedWorkloadDeploymentState,
) GeneratedWorkloadDeploymentRecord {
	t.Helper()
	record := fixture.prepare(t, fixture.authority, fixture.verification.ID)
	if state == GeneratedWorkloadDeploymentPrepared {
		return record
	}
	fixture.reserve(t, fixture.authority, GeneratedWorkloadProjectDeploymentHeadExpectation{})
	record, err := fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command,
		GeneratedWorkloadDeploymentTransition{State: GeneratedWorkloadDeploymentApplying},
	)
	if err != nil {
		t.Fatal(err)
	}
	if state == GeneratedWorkloadDeploymentApplying {
		return record
	}
	record, err = fixture.repository.TransitionGeneratedWorkloadDeployment(
		fixture.ctx, fixture.authority, fixture.command,
		GeneratedWorkloadDeploymentTransition{
			State: GeneratedWorkloadDeploymentIndeterminate, Code: "test_uncertainty",
			DetailSHA256: generatedDeploymentSHA(record.OperationID + "\x00indeterminate"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

type generatedDeploymentAuthoritySnapshot struct {
	jobStatus, stepStatus, attemptStatus string
	jobGeneration, stepGeneration        int64
	stepAttempt                          int64
	stepWorker                           string
	supersededAt                         *int64
	attemptExpiresAt                     time.Time
}

func generatedDeploymentAuthorityState(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
) generatedDeploymentAuthoritySnapshot {
	t.Helper()
	var snapshot generatedDeploymentAuthoritySnapshot
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT jobs.status,jobs.current_generation,steps.status,steps.generation,
		       steps.superseded_at_generation,steps.current_attempt,steps.worker_id,
		       attempts.status,attempts.expires_at
		FROM jobs
		JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=$2
		JOIN job_step_attempts AS attempts ON attempts.job_id=jobs.id AND
		 attempts.generation=steps.generation AND attempts.step_id=steps.id AND
		 attempts.attempt=steps.current_attempt
		WHERE jobs.id=$1
	`, fixture.jobID, fixture.authority.StepID).Scan(
		&snapshot.jobStatus, &snapshot.jobGeneration, &snapshot.stepStatus,
		&snapshot.stepGeneration, &snapshot.supersededAt, &snapshot.stepAttempt,
		&snapshot.stepWorker, &snapshot.attemptStatus, &snapshot.attemptExpiresAt,
	); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertGeneratedDeploymentAuthorityChangeRejected(
	t *testing.T,
	err error,
	fixture generatedDeploymentDatabaseFixture,
	record GeneratedWorkloadDeploymentRecord,
	state GeneratedWorkloadDeploymentState,
) {
	t.Helper()
	if !errors.Is(err, ErrGeneratedWorkloadDeploymentState) {
		t.Fatalf("authority change error=%v", err)
	}
	for _, exact := range []string{
		fmt.Sprintf("job %d", fixture.jobID), record.OperationID,
		fmt.Sprintf("generation %d", fixture.authority.Generation), string(state),
	} {
		if !strings.Contains(err.Error(), exact) {
			t.Fatalf("authority change error %q lacks %q", err, exact)
		}
	}
}
