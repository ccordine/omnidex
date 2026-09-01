package queue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

func TestWorkspaceBoundLifecycleMutationsRequireExactRepositoryAuthority(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	wrongRoot := t.TempDir()
	wrongIdentity, err := projectroot.DirectoryIdentity(wrongRoot)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("replan", func(t *testing.T) {
		job, root, identity := enqueueLifecycleWorkspaceFixture(t, repository, "replan")
		operationID := codingPlanOperationID(t, "workspace-replan", job.ID)
		command := ReplanJobCommand{
			OperationID: operationID,
			JobID:       job.ID,
			Feedback:    "Keep the same objective and revise its plan.",
		}
		requireOmittedLifecycleWorkspaceError(t, func() error {
			_, err := repository.ReplanJob(ctx, command)
			return err
		})
		requireWrongLifecycleWorkspaceError(t, func() error {
			wrong := command
			wrong.WorkspaceRoot, wrong.WorkspaceIdentity = wrongRoot, wrongIdentity
			_, err := repository.ReplanJob(ctx, wrong)
			return err
		})
		requireUnchangedLifecycleJob(t, repository, job)

		command.WorkspaceRoot, command.WorkspaceIdentity = root, identity
		result, err := repository.ReplanJob(ctx, command)
		if err != nil {
			t.Fatalf("exact replan authority: %v", err)
		}
		if !result.Applied || result.Job.ID != job.ID || result.Job.CurrentGeneration != 2 {
			t.Fatalf("exact replan result = %#v", result)
		}
		replayWithoutAuthority := command
		replayWithoutAuthority.WorkspaceRoot, replayWithoutAuthority.WorkspaceIdentity = "", ""
		requireOmittedLifecycleWorkspaceError(t, func() error {
			_, err := repository.ReplanJob(ctx, replayWithoutAuthority)
			return err
		})
	})

	t.Run("interrupt", func(t *testing.T) {
		job, root, identity := enqueueLifecycleWorkspaceFixture(t, repository, "interrupt")
		command := ReplanJobCommand{
			OperationID: codingPlanOperationID(t, "workspace-interrupt", job.ID),
			JobID:       job.ID,
			Feedback:    "Pause this exact objective for more guidance.",
		}
		requireOmittedLifecycleWorkspaceError(t, func() error {
			_, err := repository.InterruptJob(ctx, command)
			return err
		})
		requireWrongLifecycleWorkspaceError(t, func() error {
			wrong := command
			wrong.WorkspaceRoot, wrong.WorkspaceIdentity = wrongRoot, wrongIdentity
			_, err := repository.InterruptJob(ctx, wrong)
			return err
		})
		requireUnchangedLifecycleJob(t, repository, job)

		command.WorkspaceRoot, command.WorkspaceIdentity = root, identity
		result, err := repository.InterruptJob(ctx, command)
		if err != nil {
			t.Fatalf("exact interrupt authority: %v", err)
		}
		if !result.Applied || result.Job.ID != job.ID || result.Job.CurrentGeneration != 2 ||
			result.Job.Status != model.JobStatusWaiting {
			t.Fatalf("exact interrupt result = %#v", result)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		job, root, identity := enqueueLifecycleWorkspaceFixture(t, repository, "cancel")
		command := CancelJobCommand{
			OperationID: codingPlanOperationID(t, "workspace-cancel", job.ID),
			JobID:       job.ID,
			Reason:      "Cancel this exact objective.",
		}
		requireOmittedLifecycleWorkspaceError(t, func() error {
			_, err := repository.CancelJob(ctx, command)
			return err
		})
		requireWrongLifecycleWorkspaceError(t, func() error {
			wrong := command
			wrong.WorkspaceRoot, wrong.WorkspaceIdentity = wrongRoot, wrongIdentity
			_, err := repository.CancelJob(ctx, wrong)
			return err
		})
		requireUnchangedLifecycleJob(t, repository, job)

		command.WorkspaceRoot, command.WorkspaceIdentity = root, identity
		result, err := repository.CancelJob(ctx, command)
		if err != nil {
			t.Fatalf("exact cancel authority: %v", err)
		}
		if !result.Applied || result.Job.ID != job.ID || result.Job.Status != model.JobStatusCanceled {
			t.Fatalf("exact cancel result = %#v", result)
		}
	})

	t.Run("feedback", func(t *testing.T) {
		job, root, identity := enqueueLifecycleWorkspaceFixture(t, repository, "feedback")
		command := SubmitJobFeedbackCommand{
			OperationID: codingPlanOperationID(t, "workspace-feedback", job.ID),
			JobID:       job.ID,
			Feedback:    "Continue with this exact clarification.",
		}
		requireOmittedLifecycleWorkspaceError(t, func() error {
			_, err := repository.SubmitJobFeedback(ctx, command)
			return err
		})
		requireWrongLifecycleWorkspaceError(t, func() error {
			wrong := command
			wrong.WorkspaceRoot, wrong.WorkspaceIdentity = wrongRoot, wrongIdentity
			_, err := repository.SubmitJobFeedback(ctx, wrong)
			return err
		})
		requireUnchangedLifecycleJob(t, repository, job)

		command.WorkspaceRoot, command.WorkspaceIdentity = root, identity
		if _, err := repository.SubmitJobFeedback(ctx, command); !errors.Is(err, ErrStepNotWritable) {
			t.Fatalf("exact feedback authority reached error %v, want ordinary state guard %v", err, ErrStepNotWritable)
		}
	})
}

func enqueueLifecycleWorkspaceFixture(
	t *testing.T,
	repository *Repository,
	name string,
) (model.Job, string, string) {
	t.Helper()
	root := t.TempDir()
	job, err := repository.EnqueueCodingJob(
		context.Background(),
		"Exercise exact "+name+" lifecycle authority.",
		root,
	)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	required, err := repository.JobRequiresLifecycleWorkspaceAuthority(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !required {
		t.Fatal("coding job did not report required lifecycle workspace authority")
	}
	return job, root, identity
}

func requireOmittedLifecycleWorkspaceError(t *testing.T, operation func() error) {
	t.Helper()
	if err := operation(); err == nil || !strings.Contains(err.Error(), "workspace root and identity are required") {
		t.Fatalf("omitted lifecycle workspace error = %v", err)
	}
}

func requireWrongLifecycleWorkspaceError(t *testing.T, operation func() error) {
	t.Helper()
	if err := operation(); !errors.Is(err, ErrChannelSessionWorkspace) {
		t.Fatalf("wrong lifecycle workspace error = %v", err)
	}
}

func requireUnchangedLifecycleJob(t *testing.T, repository *Repository, original model.Job) {
	t.Helper()
	details, err := repository.CurrentJobDetails(context.Background(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.Status != original.Status ||
		details.Job.CurrentGeneration != original.CurrentGeneration {
		t.Fatalf("rejected lifecycle mutation changed job: before=%#v after=%#v", original, details.Job)
	}
}
