package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ExtendedRuntimeReceiptSchemaV1 = "omnidex.extended-runtime-receipt.v1"

type ExtendedEvidenceClass string

const (
	ExtendedEvidenceInProcessRuntime  ExtendedEvidenceClass = "contaminated_in_process_runtime"
	ExtendedEvidenceStructuralWitness ExtendedEvidenceClass = "contaminated_structural_witness"
	extendedCostWitnessOnly                                 = "witness_only"
)

type ExtendedRuntimeRunRequest struct {
	Surface            Surface
	RatGeneration      RatGeneration
	RuntimeFingerprint RuntimeFingerprint
	Repetition         int
	Attempt            model.StepAttemptAuthority
	Pool               *pgxpool.Pool
	Client             llm.Client
	HostStore          *labyrinthhost.Store
}

type ExtendedRuntimeProof struct {
	Kind   ExtendedSuiteProof `json:"kind"`
	SHA256 string             `json:"sha256"`
}

type ExtendedRuntimeReceipt struct {
	Schema                   string                            `json:"schema"`
	Authority                PairedRunAuthority                `json:"authority"`
	EpisodeID                string                            `json:"episode_id"`
	Seal                     queue.CognitionTerminalSeal       `json:"terminal_seal"`
	PolicyCalls              uint32                            `json:"policy_calls"`
	EnvironmentActions       uint32                            `json:"environment_actions"`
	FailedActions            uint32                            `json:"failed_actions"`
	EvidenceClass            ExtendedEvidenceClass             `json:"evidence_class"`
	PromotionEligible        bool                              `json:"promotion_eligible"`
	CancellationCode         cognitionruntime.CancellationCode `json:"cancellation_code"`
	CostBaseline             string                            `json:"cost_baseline"`
	ActualCost               int                               `json:"actual_cost"`
	WitnessCost              int                               `json:"witness_cost"`
	Proofs                   []ExtendedRuntimeProof            `json:"proofs"`
	RevisionTraceSHA256      string                            `json:"revision_trace_sha256,omitempty"`
	PrerequisiteBundleSHA256 string                            `json:"prerequisite_bundle_sha256"`
	ReceiptSHA256            string                            `json:"receipt_sha256"`
}

func (request ExtendedRuntimeRunRequest) Validate() error {
	if _, err := request.Surface.Version(); err != nil {
		return err
	}
	if err := request.RatGeneration.Validate(); err != nil {
		return err
	}
	if err := request.RuntimeFingerprint.Validate(); err != nil {
		return err
	}
	if request.Repetition <= 0 || request.Repetition > 10_000 || request.Pool == nil ||
		nilRunDependency(request.Client) || request.HostStore == nil {
		return fmt.Errorf("extended runtime requires repetition, PostgreSQL, an LLM client, and a durable host")
	}
	if request.Attempt.JobID <= 0 || request.Attempt.Generation <= 0 || request.Attempt.StepID <= 0 ||
		request.Attempt.Attempt <= 0 || request.Attempt.WorkerID == "" {
		return fmt.Errorf("extended runtime attempt authority is incomplete")
	}
	return nil
}

func (receipt ExtendedRuntimeReceipt) Validate() error {
	if receipt.Schema != ExtendedRuntimeReceiptSchemaV1 || receipt.Authority.Validate() != nil ||
		receipt.EpisodeID == "" || string(receipt.Seal.EpisodeID) != receipt.EpisodeID ||
		receipt.Seal.TraceSHA256 == "" || receipt.PolicyCalls == 0 ||
		receipt.Proofs == nil ||
		receipt.FailedActions > receipt.EnvironmentActions ||
		receipt.CostBaseline != extendedCostWitnessOnly || receipt.ActualCost < 0 ||
		receipt.WitnessCost <= 0 {
		return fmt.Errorf("extended runtime receipt authority is invalid")
	}
	completed := receipt.Seal.Outcome == queue.CognitionEpisodeCompleted
	canceled := receipt.Seal.Outcome == queue.CognitionEpisodeCanceled
	if !completed && !canceled {
		return fmt.Errorf("extended runtime receipt outcome is not registered")
	}
	if completed && (receipt.CancellationCode != "" || receipt.EnvironmentActions == 0 || receipt.ActualCost == 0) {
		return fmt.Errorf("completed extended runtime receipt contains cancellation state")
	}
	if canceled && receipt.CancellationCode != cognitionruntime.CancellationPolicyFailure &&
		receipt.CancellationCode != cognitionruntime.CancellationRunBudgetExhausted {
		return fmt.Errorf("canceled extended runtime receipt lacks a registered code")
	}
	if receipt.EvidenceClass != ExtendedEvidenceInProcessRuntime &&
		receipt.EvidenceClass != ExtendedEvidenceStructuralWitness {
		return fmt.Errorf("extended runtime evidence class is not registered")
	}
	if receipt.PromotionEligible {
		return fmt.Errorf("in-process extended runtime evidence cannot be promotion eligible")
	}
	wantKinds := []ExtendedSuiteProof{
		ProofPublicPrivateSeparation, ProofValidInvalidRails, ProofOrdinaryRuntime,
	}
	if len(receipt.Proofs) != len(wantKinds) {
		return fmt.Errorf("extended runtime receipt proof set is incomplete")
	}
	for index, proof := range receipt.Proofs {
		if proof.Kind != wantKinds[index] || !validDigest(proof.SHA256) {
			return fmt.Errorf("extended runtime receipt proof %d is invalid", index+1)
		}
	}
	revisionRequired := completed &&
		(receipt.Authority.Suite == SuiteRevise || receipt.Authority.Suite == SuiteRogue)
	if revisionRequired != validDigest(receipt.RevisionTraceSHA256) {
		return fmt.Errorf("extended runtime revision trace authority is inconsistent")
	}
	rogue := receipt.Authority.Suite == SuiteRogue
	if rogue != validDigest(receipt.PrerequisiteBundleSHA256) {
		return fmt.Errorf("extended runtime prerequisite authority is inconsistent")
	}
	want, err := extendedRuntimeReceiptSHA(receipt)
	if err != nil || receipt.ReceiptSHA256 != want {
		return fmt.Errorf("extended runtime receipt hash changed")
	}
	return nil
}

func extendedRuntimeReceiptSHA(receipt ExtendedRuntimeReceipt) (string, error) {
	receipt.ReceiptSHA256 = ""
	return digestJSON(receipt)
}

type contaminatedExtendedClient interface {
	ExtendedEvidenceClass() ExtendedEvidenceClass
}

func extendedClientEvidenceClass(client llm.Client) (ExtendedEvidenceClass, error) {
	classified, contaminated := client.(contaminatedExtendedClient)
	if !contaminated {
		return ExtendedEvidenceInProcessRuntime, nil
	}
	if classified.ExtendedEvidenceClass() != ExtendedEvidenceStructuralWitness {
		return "", fmt.Errorf("extended runtime client declared an invalid evidence class")
	}
	return ExtendedEvidenceStructuralWitness, nil
}
