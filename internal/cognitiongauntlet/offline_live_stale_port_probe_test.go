package cognitiongauntlet

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestLiveStalePortWrappersPauseBeforeExactProductionPortAndSealRejection(t *testing.T) {
	for _, port := range liveStalePorts() {
		t.Run(string(port), func(t *testing.T) {
			checkpointPath := filepath.Join(t.TempDir(), "checkpoint.json")
			rejectionPath := filepath.Join(filepath.Dir(checkpointPath), "rejection.json")
			attempt := model.StepAttemptAuthority{
				JobID: 41, Generation: 2, StepID: 7, Attempt: 3, WorkerID: "expired-worker",
			}
			probe, err := newLiveStalePortController(
				port, attempt, checkpointPath, rejectionPath,
			)
			if err != nil {
				t.Fatal(err)
			}
			paused := 0
			probe.pause = func() error { paused++; return nil }
			invokeLiveStalePort(t, probe, port)
			if paused != 1 {
				t.Fatalf("pause calls=%d, want 1", paused)
			}
			checkpoint, err := loadLiveStalePortCheckpoint(checkpointPath)
			if err != nil {
				t.Fatal(err)
			}
			rejection, err := loadLiveStalePortRejection(rejectionPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := rejection.ValidateFor(checkpoint); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLiveStalePolicyRejectionPreservesIndeterminateProviderWrite(t *testing.T) {
	rejection := liveStalePortRejection{
		Schema: liveStalePortRejectionSchemaV2, Port: liveStalePolicyFinish,
		PID: 817, Attempt: model.StepAttemptAuthority{
			JobID: 41, Generation: 2, StepID: 7, Attempt: 3, WorkerID: "expired-worker",
		},
		CommandSHA256:              strings.Repeat("a", 64),
		ErrorClass:                 liveStaleErrorAttempt,
		ProviderRequestDisposition: llm.ProviderRequestWriteIndeterminate,
		RejectedAt:                 time.Now().UTC(),
	}
	if err := rejection.Validate(); err != nil {
		t.Fatalf("indeterminate stale provider write was not preserved: %v", err)
	}
	rejection.ProviderRequestDisposition = llm.ProviderRequestNotDispatched
	if err := rejection.Validate(); err == nil {
		t.Fatal("non-dispatched stale request was accepted as provider-reaching")
	}
}

func invokeLiveStalePort(t *testing.T, probe *liveStalePortController, port liveStalePort) {
	t.Helper()
	ctx := context.Background()
	switch port {
	case liveStalePolicyFinish:
		journal := liveStaleCallJournal{probe: probe, base: staleTestJournal{}}
		result := cognitionpolicy.CallResult{
			ProviderRequestDisposition: llm.ProviderRequestDispatched,
			ProviderUsagePresent:       true,
			ProviderDoneReason:         "stop", ProviderUsage: llm.ProviderGenerationUsage{
				PromptEvalCount: 2, EvalCount: 1, TotalDurationNanos: 4,
				LoadDurationNanos: 1, PromptEvalDurationNanos: 1, EvalDurationNanos: 1,
			},
		}
		err := journal.Finish(ctx, cognitionpolicy.CallAttempt{}, result, cognitionpolicy.CallEvidence{})
		if !errors.Is(err, queue.ErrStaleStepAttempt) {
			t.Fatalf("Finish() error=%v, want stale attempt", err)
		}
	case liveStaleReconcile:
		reconciler := liveStaleReconciler{probe: probe, base: staleTestReconciler{}}
		_, err := reconciler.Reconcile(ctx, cognitionruntime.ReconciliationCommand{})
		if !errors.Is(err, queue.ErrStaleStepAttempt) {
			t.Fatalf("Reconcile() error=%v, want stale attempt", err)
		}
	case liveStaleEnvironmentApply:
		environment := liveStaleEnvironment{probe: probe, base: staleTestEnvironment{}}
		_, err := environment.Apply(ctx, cognition.EpisodeRef{}, cognition.WorldRevision{}, cognition.RegisteredAction{})
		if !errors.Is(err, cognition.ErrAuthorityDenied) {
			t.Fatalf("Apply() error=%v, want authority denied", err)
		}
	case liveStaleTerminal:
		episodes := liveStaleEpisodeJournal{probe: probe, base: staleTestEpisodes{}}
		_, err := episodes.AdvanceSatisfied(ctx, cognitionruntime.CompletionCommand{})
		if !errors.Is(err, queue.ErrStaleStepAttempt) {
			t.Fatalf("AdvanceSatisfied() error=%v, want stale attempt", err)
		}
	default:
		t.Fatalf("unregistered test port %q", port)
	}
}

type staleTestJournal struct{}

func (staleTestJournal) Start(context.Context, cognitionpolicy.CallAttempt) (cognitionpolicy.CallReservation, error) {
	return cognitionpolicy.CallReservation{}, nil
}

func (staleTestJournal) Finish(
	context.Context, cognitionpolicy.CallAttempt, cognitionpolicy.CallResult, cognitionpolicy.CallEvidence,
) error {
	return queue.ErrStaleStepAttempt
}

type staleTestReconciler struct{}

func (staleTestReconciler) Reconcile(
	context.Context, cognitionruntime.ReconciliationCommand,
) (cognitionruntime.ReconciliationReceipt, error) {
	return cognitionruntime.ReconciliationReceipt{}, queue.ErrStaleStepAttempt
}

type staleTestEnvironment struct{}

func (staleTestEnvironment) Start(context.Context, cognition.ScenarioRef) (cognition.Transition, error) {
	return cognition.Transition{}, nil
}

func (staleTestEnvironment) Apply(
	context.Context, cognition.EpisodeRef, cognition.WorldRevision, cognition.RegisteredAction,
) (cognition.Transition, error) {
	return cognition.Transition{}, cognition.ErrAuthorityDenied
}

type staleTestEpisodes struct{}

func (staleTestEpisodes) TerminalProgress(
	context.Context, cognitionruntime.Binding,
) (*cognitionruntime.EpisodeProgress, error) {
	return nil, nil
}

func (staleTestEpisodes) AdvanceSatisfied(
	context.Context, cognitionruntime.CompletionCommand,
) (cognitionruntime.EpisodeProgress, error) {
	return cognitionruntime.EpisodeProgress{}, queue.ErrStaleStepAttempt
}

func (staleTestEpisodes) FailTerminal(
	context.Context, cognitionruntime.CompletionCommand,
) (cognitionruntime.EpisodeProgress, error) {
	return cognitionruntime.EpisodeProgress{}, queue.ErrStaleStepAttempt
}
