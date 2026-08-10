package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestTaskCommandLifecycleRequiresMatchingJobAndLedgerAuthority(t *testing.T) {
	active := taskLedgerHeader{
		Owner:     taskstate.LedgerOwner{Kind: taskstate.OwnerJob, JobID: 41},
		JobStatus: model.JobStatusRunning, Status: taskstate.LedgerActive,
	}
	if err := validateTaskCommandJobLifecycle(active, taskstate.AddEdgeCommand{}); err != nil {
		t.Fatalf("active command rejected: %v", err)
	}
	if err := validateTaskCommandJobLifecycle(active, taskstate.CloseLedgerCommand{
		Status: taskstate.LedgerClosed,
	}); !errors.Is(err, taskstate.ErrInvalidCommand) {
		t.Fatalf("nonterminal job close error=%v", err)
	}

	completed := active
	completed.JobStatus = model.JobStatusCompleted
	if err := validateTaskCommandJobLifecycle(completed, taskstate.CloseLedgerCommand{
		Status: taskstate.LedgerClosed,
	}); err != nil {
		t.Fatalf("matching terminalization rejected: %v", err)
	}
	for name, command := range map[string]taskstate.Command{
		"ordinary mutation": taskstate.AddEdgeCommand{},
		"wrong close":       taskstate.CloseLedgerCommand{Status: taskstate.LedgerFailed},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateTaskCommandJobLifecycle(completed, command); !errors.Is(err, taskstate.ErrInvalidState) {
				t.Fatalf("terminal mismatch error=%v", err)
			}
		})
	}

	alreadyClosed := completed
	alreadyClosed.Status = taskstate.LedgerClosed
	if err := validateTaskCommandJobLifecycle(alreadyClosed, taskstate.AddEdgeCommand{}); !errors.Is(err, taskstate.ErrInvalidState) {
		t.Fatalf("closed ledger mutation error=%v", err)
	}
	inconsistent := active
	inconsistent.Status = taskstate.LedgerFailed
	if err := validateTaskCommandJobLifecycle(inconsistent, taskstate.AddEdgeCommand{}); !errors.Is(err, taskstate.ErrInvalidState) {
		t.Fatalf("nonterminal job/ledger mismatch error=%v", err)
	}
}
