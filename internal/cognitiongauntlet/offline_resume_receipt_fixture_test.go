package cognitiongauntlet

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func testLiveStaleProbe(t *testing.T, now time.Time) LiveStaleProbeReceipt {
	t.Helper()
	proofs := make([]LiveStalePortProof, 0, len(liveStalePorts()))
	for index, port := range liveStalePorts() {
		original := model.StepAttemptAuthority{
			JobID: int64(100 + index), Generation: 1, StepID: int64(200 + index),
			Attempt: 1, WorkerID: fmt.Sprintf("old-%d", index),
		}
		replacement := original
		replacement.Attempt++
		replacement.WorkerID = fmt.Sprintf("new-%d", index)
		entered := now.Add(time.Duration(index*10) * time.Second)
		commandSHA := fmt.Sprintf("%064x", index+1)
		episode := cognition.EpisodeRef{ID: cognition.EpisodeID(fmt.Sprintf("probe-%d", index))}
		state := LiveStaleDurableState{
			Episode: episode, TraceSHA256: fmt.Sprintf("%064x", index+20),
			SealSHA256: fmt.Sprintf("%064x", index+10), GraphVersion: 2,
			LedgerVersion: 3, WorkingSetVersion: 4, PolicyResults: 7,
			ReconciliationReceipts: 7, ActionRecords: 7, WorkingSetEvents: 3,
			ProgressRecords: 1, HostReceipts: 7,
			HostCurrent: cognition.WorldRevision{
				EpisodeID: episode.ID, Number: 8, SHA256: fmt.Sprintf("%064x", index+30),
			}, HostTerminal: true,
		}
		stateSHA, err := state.SHA256()
		if err != nil {
			t.Fatal(err)
		}
		proofs = append(proofs, LiveStalePortProof{
			Port: port, Episode: episode, EpisodeSealSHA256: fmt.Sprintf("%064x", index+10),
			EvaluationSHA256:       fmt.Sprintf("%064x", index+40),
			PromotionReceiptSHA256: fmt.Sprintf("%064x", index+50),
			DatabaseSchema:         fmt.Sprintf("runtime_%d", index), HostSchema: fmt.Sprintf("host_%d", index),
			Original: original, Replacement: replacement,
			OriginalPID: 300 + index, ReplacementPID: 400 + index,
			Checkpoint: liveStalePortCheckpoint{
				Schema: liveStalePortCheckpointSchemaV1, Port: port, PID: 300 + index,
				Attempt: original, CommandSHA256: commandSHA, EnteredAt: entered,
			},
			Rejection: liveStalePortRejection{
				Schema: liveStalePortRejectionSchemaV2, Port: port, PID: 300 + index,
				Attempt: original, CommandSHA256: commandSHA, ErrorClass: port.expectedError(),
				ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
				RejectedAt:                 entered.Add(3 * time.Second),
			},
			StateBefore: state, StateAfter: state,
			StateBeforeSHA256: stateSHA, StateAfterSHA256: stateSHA,
			ReplacementSealedAt:  entered.Add(time.Second),
			OriginalResumedAt:    entered.Add(2 * time.Second),
			InferenceStartedAt:   entered.Add(-time.Second),
			InferenceExitedAt:    entered.Add(4 * time.Second),
			EvaluatorStartedAt:   entered.Add(100 * time.Second),
			EvaluatorCompletedAt: entered.Add(101 * time.Second),
		})
		if port == liveStalePolicyFinish {
			proof := &proofs[len(proofs)-1]
			proof.StateBefore.PolicyAbandonments = 1
			proof.StateAfter.PolicyAbandonments = 1
			proof.StateBeforeSHA256, err = proof.StateBefore.SHA256()
			if err != nil {
				t.Fatal(err)
			}
			proof.StateAfterSHA256 = proof.StateBeforeSHA256
			proof.Rejection.ProviderRequestDisposition = llm.ProviderRequestDispatched
			proof.Rejection.ProviderUsagePresent = true
			proof.Rejection.ProviderDoneReason = "stop"
			proof.Rejection.ProviderUsage = llm.ProviderGenerationUsage{
				PromptEvalCount: 2, EvalCount: 1, TotalDurationNanos: 4,
				LoadDurationNanos: 1, PromptEvalDurationNanos: 1, EvalDurationNanos: 1,
			}
		}
	}
	receipt := LiveStaleProbeReceipt{
		Schema:                   LiveStaleProbeReceiptSchemaV2,
		PublicRunAuthoritySHA256: strings.Repeat("1", 64),
		Probes:                   proofs, CompletedAt: proofs[len(proofs)-1].EvaluatorCompletedAt,
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	return receipt
}

func testResumeInterruption(
	t *testing.T,
	boundary uint32,
	attempt int64,
	baseline ResumeBaselineArtifact,
) OfflineResumeInterruptionReceipt {
	t.Helper()
	checkpoint, err := baseline.checkpoint(boundary)
	if err != nil {
		t.Fatal(err)
	}
	before := checkpoint.PreCall
	before.Bound.Attempt.Attempt = uint64(attempt)
	before.Bound.Attempt.WorkerID = fmt.Sprintf("worker-%d", attempt)
	before.Bound.Projection.ID = cognition.ContextProjectionID(fmt.Sprintf("before-%d", attempt))
	before.Bound.SnapshotSHA256 = strings.Repeat("a", 64)
	after := before
	after.Bound.Attempt.Attempt++
	after.Bound.Attempt.WorkerID = fmt.Sprintf("worker-%d", attempt+1)
	after.Bound.Projection.ID = cognition.ContextProjectionID(fmt.Sprintf("after-%d", attempt))
	after.Bound.SnapshotSHA256 = strings.Repeat("b", 64)
	continuity, err := NewTakeoverContinuityProof(before, after)
	if err != nil {
		t.Fatal(err)
	}
	checkpointSHA, err := digestJSON(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	receipt := OfflineResumeInterruptionReceipt{
		DecisionBoundary: boundary, BaselineCheckpointSHA256: checkpointSHA,
		Original: model.StepAttemptAuthority{
			JobID: 71, Generation: 4, StepID: 9, Attempt: attempt,
			WorkerID: before.Bound.Attempt.WorkerID,
		},
		Replacement: model.StepAttemptAuthority{
			JobID: 71, Generation: 4, StepID: 9, Attempt: attempt + 1,
			WorkerID: after.Bound.Attempt.WorkerID,
		},
		OriginalPID: 100 + int(attempt), ReplacementPID: 101 + int(attempt),
		OriginalDiedAt: time.Now().UTC(), Continuity: continuity,
	}
	if boundary == 0 {
		receipt.OriginalDiedAt = time.Time{}
		receipt.OriginalStoppedAt = time.Now().UTC()
		receipt.OriginalResumedAt = receipt.OriginalStoppedAt.Add(time.Second)
		receipt.OriginalExitedAt = receipt.OriginalResumedAt.Add(time.Second)
	}
	return receipt
}
