package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
)

const LiveStaleProbeReceiptSchemaV2 = "omnidex.live-stale-probe-receipt.v2"

type LiveStaleDurableState struct {
	Episode                cognition.EpisodeRef    `json:"episode"`
	TraceSHA256            string                  `json:"trace_sha256"`
	SealSHA256             string                  `json:"seal_sha256"`
	GraphVersion           uint64                  `json:"graph_version"`
	LedgerVersion          uint64                  `json:"ledger_version"`
	WorkingSetVersion      uint64                  `json:"working_set_version"`
	PolicyResults          int                     `json:"policy_results"`
	PolicyAbandonments     int                     `json:"policy_abandonments"`
	ReconciliationReceipts int                     `json:"reconciliation_receipts"`
	ActionRecords          int                     `json:"action_records"`
	WorkingSetEvents       int                     `json:"working_set_events"`
	ProgressRecords        int                     `json:"progress_records"`
	HostReceipts           int                     `json:"host_receipts"`
	HostCurrent            cognition.WorldRevision `json:"host_current"`
	HostTerminal           bool                    `json:"host_terminal"`
}

type LiveStalePortProof struct {
	Port                   liveStalePort              `json:"port"`
	Episode                cognition.EpisodeRef       `json:"episode"`
	EpisodeSealSHA256      string                     `json:"episode_seal_sha256"`
	EvaluationSHA256       string                     `json:"evaluation_sha256"`
	PromotionReceiptSHA256 string                     `json:"promotion_receipt_sha256"`
	DatabaseSchema         string                     `json:"database_schema"`
	HostSchema             string                     `json:"host_schema"`
	Original               model.StepAttemptAuthority `json:"original_attempt"`
	Replacement            model.StepAttemptAuthority `json:"replacement_attempt"`
	OriginalPID            int                        `json:"original_pid"`
	ReplacementPID         int                        `json:"replacement_pid"`
	Checkpoint             liveStalePortCheckpoint    `json:"checkpoint"`
	Rejection              liveStalePortRejection     `json:"rejection"`
	StateBefore            LiveStaleDurableState      `json:"state_before"`
	StateAfter             LiveStaleDurableState      `json:"state_after"`
	StateBeforeSHA256      string                     `json:"state_before_sha256"`
	StateAfterSHA256       string                     `json:"state_after_sha256"`
	ReplacementSealedAt    time.Time                  `json:"replacement_sealed_at"`
	OriginalResumedAt      time.Time                  `json:"original_resumed_at"`
	InferenceStartedAt     time.Time                  `json:"inference_started_at"`
	InferenceExitedAt      time.Time                  `json:"inference_exited_at"`
	EvaluatorStartedAt     time.Time                  `json:"evaluator_started_at"`
	EvaluatorCompletedAt   time.Time                  `json:"evaluator_completed_at"`
}

type LiveStaleProbeReceipt struct {
	Schema                   string               `json:"schema"`
	PublicRunAuthoritySHA256 string               `json:"public_run_authority_sha256"`
	Probes                   []LiveStalePortProof `json:"probes"`
	CompletedAt              time.Time            `json:"completed_at"`
}

func (state LiveStaleDurableState) Validate() error {
	if state.Episode.Validate() != nil || !validDigest(state.TraceSHA256) ||
		!validDigest(state.SealSHA256) || state.GraphVersion == 0 ||
		state.LedgerVersion == 0 || state.WorkingSetVersion == 0 ||
		state.PolicyResults < 1 || state.PolicyAbandonments < 0 || state.PolicyAbandonments > 1 ||
		state.ActionRecords < 0 || state.WorkingSetEvents < 0 ||
		state.ProgressRecords < 1 || state.HostReceipts < 0 ||
		state.HostCurrent.Validate() != nil || state.HostCurrent.EpisodeID != state.Episode.ID {
		return fmt.Errorf("live stale durable state is invalid")
	}
	return nil
}

func (state LiveStaleDurableState) SHA256() (string, error) {
	if err := state.Validate(); err != nil {
		return "", err
	}
	return digestJSON(state)
}

func (proof LiveStalePortProof) Validate() error {
	if proof.Port.Validate() != nil || proof.Episode.Validate() != nil ||
		!validDigest(proof.EpisodeSealSHA256) || !validDigest(proof.EvaluationSHA256) ||
		!validDigest(proof.PromotionReceiptSHA256) ||
		requireExact(proof.DatabaseSchema, "live stale runtime schema", 128) != nil ||
		requireExact(proof.HostSchema, "live stale host schema", 128) != nil ||
		proof.DatabaseSchema == proof.HostSchema || !validTakeoverAttempt(proof.Original) ||
		!validTakeoverAttempt(proof.Replacement) || proof.OriginalPID <= 0 ||
		proof.ReplacementPID <= 0 || proof.OriginalPID == proof.ReplacementPID ||
		proof.Checkpoint.Validate() != nil || proof.Rejection.ValidateFor(proof.Checkpoint) != nil ||
		proof.Checkpoint.Port != proof.Port || proof.Checkpoint.Attempt != proof.Original ||
		proof.Checkpoint.PID != proof.OriginalPID ||
		proof.Original.JobID != proof.Replacement.JobID ||
		proof.Original.Generation != proof.Replacement.Generation ||
		proof.Original.StepID != proof.Replacement.StepID ||
		proof.Replacement.Attempt != proof.Original.Attempt+1 ||
		proof.Replacement.WorkerID == proof.Original.WorkerID ||
		proof.StateBefore.Validate() != nil || !reflect.DeepEqual(proof.StateBefore, proof.StateAfter) ||
		proof.StateBefore.Episode != proof.Episode ||
		proof.StateBefore.SealSHA256 != proof.EpisodeSealSHA256 ||
		!validDigest(proof.StateBeforeSHA256) || proof.StateAfterSHA256 != proof.StateBeforeSHA256 ||
		proof.ReplacementSealedAt.IsZero() ||
		proof.OriginalResumedAt.Before(proof.ReplacementSealedAt) ||
		proof.Rejection.RejectedAt.Before(proof.OriginalResumedAt) ||
		proof.InferenceStartedAt.IsZero() ||
		proof.InferenceExitedAt.Before(proof.InferenceStartedAt) ||
		proof.EvaluatorStartedAt.Before(proof.InferenceExitedAt) ||
		proof.EvaluatorCompletedAt.Before(proof.EvaluatorStartedAt) {
		return fmt.Errorf("live stale-port proof is invalid")
	}
	beforeSHA, err := proof.StateBefore.SHA256()
	if err != nil || beforeSHA != proof.StateBeforeSHA256 {
		return fmt.Errorf("live stale-port before-state digest is invalid")
	}
	afterSHA, err := proof.StateAfter.SHA256()
	if err != nil || afterSHA != proof.StateAfterSHA256 {
		return fmt.Errorf("live stale-port after-state digest is invalid")
	}
	if proof.Port == liveStalePolicyFinish {
		if proof.StateBefore.PolicyAbandonments != 1 ||
			!proof.Rejection.ProviderRequestDispatched {
			return fmt.Errorf("live policy-finish proof lacks one exact abandoned/dispatched call")
		}
	} else if proof.StateBefore.PolicyAbandonments != 0 {
		return fmt.Errorf("nonpolicy stale-port proof carries a policy abandonment")
	}
	return nil
}

func (receipt LiveStaleProbeReceipt) Validate() error {
	if receipt.Schema != LiveStaleProbeReceiptSchemaV2 ||
		!validDigest(receipt.PublicRunAuthoritySHA256) || receipt.Probes == nil ||
		len(receipt.Probes) != len(liveStalePorts()) || receipt.CompletedAt.IsZero() {
		return fmt.Errorf("live stale-probe receipt is invalid")
	}
	seen := make(map[liveStalePort]struct{}, len(receipt.Probes))
	latestRejection, latestCompletion := time.Time{}, time.Time{}
	for _, proof := range receipt.Probes {
		if err := proof.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[proof.Port]; duplicate {
			return fmt.Errorf("live stale-probe receipt repeats port %q", proof.Port)
		}
		seen[proof.Port] = struct{}{}
		if proof.Rejection.RejectedAt.After(latestRejection) {
			latestRejection = proof.Rejection.RejectedAt
		}
		if proof.EvaluatorCompletedAt.After(latestCompletion) {
			latestCompletion = proof.EvaluatorCompletedAt
		}
	}
	if receipt.CompletedAt != latestCompletion || receipt.CompletedAt.Before(latestRejection) {
		return fmt.Errorf("live stale-probe receipt completion is not derived from its proofs")
	}
	for _, port := range liveStalePorts() {
		if _, exists := seen[port]; !exists {
			return fmt.Errorf("live stale-probe receipt omitted port %q", port)
		}
	}
	return nil
}

func (receipt LiveStaleProbeReceipt) Complete() bool {
	return receipt.Validate() == nil && liveStaleWriteClassCount(receipt) == 5
}

func SealLiveStaleProbeReceipt(path string, receipt LiveStaleProbeReceipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, receipt, "live stale-probe receipt")
}

func LoadLiveStaleProbeReceipt(path string) (LiveStaleProbeReceipt, error) {
	var receipt LiveStaleProbeReceipt
	if err := loadStrictJSONFile(path, &receipt, "live stale-probe receipt"); err != nil {
		return LiveStaleProbeReceipt{}, err
	}
	if err := receipt.Validate(); err != nil {
		return LiveStaleProbeReceipt{}, err
	}
	return receipt, nil
}
