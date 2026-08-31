package queue

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestFreshSchemaVerificationCommandEvidenceIsExactTerminalAndImmutable(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	pool, repository := freshEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise exact command evidence", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "command-evidence-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}

	started := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	zero, nine := 0, 9
	commands := []VerificationCommandEvidence{
		verificationIntegrationCommand(claim.Authority, 1, started, &zero, ""),
		verificationIntegrationCommand(claim.Authority, 2, started.Add(time.Second), &nine, ""),
		verificationIntegrationCommand(claim.Authority, 3, started.Add(2*time.Second), nil, "executable not found"),
	}
	commands[0].Environment = nil
	commands[0].Stdin = []byte{}
	commands[0].Stdout = []byte{}
	commands[0].Stderr = []byte{}
	changed := verificationIntegrationCommand(claim.Authority, 4, started.Add(3*time.Second), &zero, "")
	changed.Phase = VerificationHostFinal
	changed.WorkspaceSHA256Before = strings.Repeat("d", 64)
	changed.WorkspaceSHA256After = strings.Repeat("e", 64)
	commands = append(commands, changed)
	observation := verificationIntegrationCommand(claim.Authority, 5, started.Add(4*time.Second), &zero, "")
	observation.Phase = VerificationIsolatedFinal
	observation.WorkspaceSHA256Before = strings.Repeat("f", 64)
	observation.ObservationError = "authoritative post-command workspace hash failed"
	commands = append(commands, observation)
	for _, command := range commands {
		if err := repository.AppendVerificationCommandEvidence(ctx, command); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := pool.Exec(ctx, invalidObservationCommandSQL(false), job.ID); err == nil {
		t.Fatal("observation failure with incomplete process output was accepted")
	}
	if _, err := pool.Exec(ctx, invalidObservationCommandSQL(true), job.ID, strings.Repeat("x", 8193)); err == nil {
		t.Fatal("observation failure with an unbounded launch error was accepted")
	}
	if err := repository.AppendVerificationCommandEvidence(ctx, commands[2]); err == nil {
		t.Fatal("duplicate verification ordinal was accepted")
	}
	wrongWorker := verificationIntegrationCommand(claim.Authority, 6, started.Add(5*time.Second), &zero, "")
	wrongWorker.Authority.WorkerID = "different-worker"
	if err := repository.AppendVerificationCommandEvidence(ctx, wrongWorker); err == nil {
		t.Fatal("verification attempt worker mismatch was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE verification_command_evidence SET stdout='tampered' WHERE job_id=$1`, job.ID); err == nil {
		t.Fatal("verification evidence update was accepted")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM verification_command_evidence WHERE job_id=$1`, job.ID); err == nil {
		t.Fatal("verification evidence delete was accepted")
	}

	stored, err := repository.ListVerificationCommandEvidenceForJob(ctx, job.ID, 0, 10)
	if err != nil || len(stored) != 5 ||
		stored[0].Status != VerificationCommandSucceeded ||
		stored[1].Status != VerificationCommandExitFailed ||
		stored[2].Status != VerificationCommandLaunchFailed ||
		stored[3].Status != VerificationCommandWorkspaceChanged ||
		stored[4].Status != VerificationCommandObservationFailed {
		t.Fatalf("commands=%#v err=%v", stored, err)
	}
	if stored[0].Environment == nil || !stored[0].StdinPresent ||
		stored[0].Stdin == nil || stored[0].Stdout == nil || stored[0].Stderr == nil {
		t.Fatalf("present empty command bytes were not preserved: %#v", stored[0])
	}
	firstPage, err := repository.ListVerificationCommandEvidenceForJob(ctx, job.ID, 0, 2)
	if err != nil || len(firstPage) != 2 {
		t.Fatalf("first verification-command page=%#v err=%v", firstPage, err)
	}
	secondPage, err := repository.ListVerificationCommandEvidenceForJob(ctx, job.ID, firstPage[1].ID, 2)
	if err != nil || len(secondPage) != 2 {
		t.Fatalf("second verification-command page=%#v err=%v", secondPage, err)
	}
	thirdPage, err := repository.ListVerificationCommandEvidenceForJob(ctx, job.ID, secondPage[1].ID, 2)
	if err != nil || len(thirdPage) != 1 {
		t.Fatalf("third verification-command page=%#v err=%v", thirdPage, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_step_attempts SET status='completed',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID); err == nil {
		t.Fatal("step completion accepted failed verification command evidence")
	}
}

func verificationIntegrationCommand(
	authority model.StepAttemptAuthority,
	ordinal int64,
	started time.Time,
	exitCode *int,
	launchError string,
) VerificationCommandEvidence {
	return VerificationCommandEvidence{
		Authority: authority, Phase: VerificationIsolatedTask, Ordinal: ordinal,
		Argv: []string{"go", "test", "./..."}, Environment: []string{"GOCACHE=/tmp/cache"},
		WorkingDirectory: "/tmp/evidence-project", StartedAt: started,
		FinishedAt: started.Add(10 * time.Millisecond), ExitCode: exitCode,
		LaunchError: launchError, Stdout: []byte("output\n"), StdoutComplete: true,
		Stderr: []byte("diagnostic\n"), StderrComplete: true,
	}
}

func invalidObservationCommandSQL(unboundedLaunchError bool) string {
	exitCode := "exit_code"
	launchError := "launch_error"
	stdoutComplete := "false"
	if unboundedLaunchError {
		exitCode = "NULL"
		launchError = "$2"
		stdoutComplete = "stdout_complete"
	}
	return `
		INSERT INTO verification_command_evidence (
			job_id,generation,step_id,step_attempt,worker_id,phase,ordinal,
			argv,argv_sha256,environment,environment_sha256,
			stdin_present,stdin,stdin_sha256,working_directory,
			started_at,finished_at,duration_nanos,exit_code,launch_error,observation_error,
			stdout,stdout_complete,stdout_sha256,stderr,stderr_complete,stderr_sha256,
			workspace_sha256_before,workspace_sha256_after,status
		)
		SELECT job_id,generation,step_id,step_attempt,worker_id,phase,6,
		       argv,argv_sha256,environment,environment_sha256,
		       stdin_present,stdin,stdin_sha256,working_directory,
		       started_at,finished_at,duration_nanos,` + exitCode + `,` + launchError + `,observation_error,
		       stdout,` + stdoutComplete + `,stdout_sha256,stderr,stderr_complete,stderr_sha256,
		       workspace_sha256_before,workspace_sha256_after,status
		FROM verification_command_evidence
		WHERE job_id=$1 AND ordinal=5
	`
}
