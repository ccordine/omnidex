package queue

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestFreshSchemaVerificationCommandResultsDoNotVetoLaterCompletion(t *testing.T) {
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
	commands[0].ContainerID = strings.Repeat("a", 64)
	commands[0].ContainerImageID = "sha256:" + strings.Repeat("b", 64)
	commands[0].ContainerExecID = strings.Repeat("c", 64)
	networkEnabled := false
	commands[0].ContainerNetworkEnabled = &networkEnabled
	changed := verificationIntegrationCommand(claim.Authority, 4, started.Add(3*time.Second), &zero, "")
	changed.Phase = VerificationHostFinal
	commands = append(commands, changed)
	observation := verificationIntegrationCommand(claim.Authority, 5, started.Add(4*time.Second), &zero, "")
	observation.Phase = VerificationIsolatedFinal
	observation.ObservationError = "host working directory was removed"
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

	stored, err := repository.ListVerificationCommandEvidenceForJob(ctx, job.ID, 0, 10)
	if err != nil || len(stored) != 5 ||
		stored[0].Status != VerificationCommandSucceeded ||
		stored[1].Status != VerificationCommandExitFailed ||
		stored[2].Status != VerificationCommandLaunchFailed ||
		stored[3].Status != VerificationCommandSucceeded ||
		stored[4].Status != VerificationCommandObservationFailed {
		t.Fatalf("commands=%#v err=%v", stored, err)
	}
	if stored[0].Environment == nil || !stored[0].StdinPresent ||
		stored[0].Stdin == nil || stored[0].Stdout == nil || stored[0].Stderr == nil {
		t.Fatalf("present empty command bytes were not preserved: %#v", stored[0])
	}
	if stored[0].ContainerID != commands[0].ContainerID || stored[0].ContainerImageID != commands[0].ContainerImageID || stored[0].ContainerExecID != commands[0].ContainerExecID {
		t.Fatalf("exact Docker command identities were not preserved: %#v", stored[0])
	}
	if stored[0].ContainerNetworkEnabled == nil || *stored[0].ContainerNetworkEnabled {
		t.Fatal("exact disabled network observation was not preserved")
	}
	for _, mutation := range []string{"container_image_id=NULL", "container_image_id='node:22'", "container_exec_id=NULL", "container_id=NULL", "container_network_enabled=NULL", "container_network_enabled=true", "working_directory='/tmp/source'", "phase='host_cleanup'", "container_id=NULL,container_image_id=NULL,container_exec_id=NULL,container_network_enabled=NULL"} {
		if _, err := pool.Exec(ctx, "UPDATE verification_command_evidence SET "+mutation+" WHERE job_id=$1 AND ordinal=1", job.ID); err == nil {
			t.Fatalf("schema accepted invalid Docker authority: %s", mutation)
		}
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
	passed := verificationIntegrationCommand(claim.Authority, 6, started.Add(5*time.Second), &zero, "")
	passed.Phase = VerificationIsolatedInstall
	acquiring := true
	passed.ContainerNetworkEnabled = &acquiring
	if err := repository.AppendVerificationCommandEvidence(ctx, passed); err != nil {
		t.Fatal(err)
	}
	latest, err := repository.ListVerificationCommandEvidenceForJob(ctx, job.ID, stored[len(stored)-1].ID, 1)
	if err != nil || len(latest) != 1 || latest[0].ContainerNetworkEnabled == nil || !*latest[0].ContainerNetworkEnabled {
		t.Fatalf("acquisition observation did not round-trip: %+v %v", latest, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_step_attempts SET status='completed',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID); err != nil {
		t.Fatalf("historical command failure vetoed code-owned completion: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM verification_command_evidence WHERE job_id=$1`, job.ID); err != nil {
		t.Fatalf("command log cannot be discarded: %v", err)
	}
}

func verificationIntegrationCommand(
	authority model.StepAttemptAuthority,
	ordinal int64,
	started time.Time,
	exitCode *int,
	launchError string,
) VerificationCommandEvidence {
	networkEnabled := false
	return VerificationCommandEvidence{
		Authority: authority, Phase: VerificationIsolatedTask, Ordinal: ordinal,
		Argv: []string{"go", "test", "./..."}, Environment: []string{"GOCACHE=/tmp/cache"},
		WorkingDirectory: "/workspace", StartedAt: started,
		ContainerID: strings.Repeat("a", 64), ContainerImageID: "sha256:" + strings.Repeat("b", 64),
		ContainerExecID: strings.Repeat("c", 64), ContainerNetworkEnabled: &networkEnabled,
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
			argv,environment,stdin_present,stdin,working_directory,
			container_id,container_image_id,container_exec_id,container_network_enabled,
			started_at,finished_at,duration_nanos,exit_code,launch_error,observation_error,
			stdout,stdout_complete,stderr,stderr_complete,status
		)
		SELECT job_id,generation,step_id,step_attempt,worker_id,phase,6,
		       argv,environment,stdin_present,stdin,working_directory,
		       container_id,container_image_id,container_exec_id,container_network_enabled,
		       started_at,finished_at,duration_nanos,` + exitCode + `,` + launchError + `,observation_error,
		       stdout,` + stdoutComplete + `,stderr,stderr_complete,status
		FROM verification_command_evidence
		WHERE job_id=$1 AND ordinal=5
	`
}
