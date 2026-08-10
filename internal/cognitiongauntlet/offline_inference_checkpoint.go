package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

const PausedInferenceCheckpointSchemaV1 = "omnidex.paused-cognition-inference.v1"

type RuntimePrefix struct {
	Cycles                  uint32 `json:"cycles"`
	PolicyCalls             uint32 `json:"policy_calls"`
	RecoveredDecisions      uint32 `json:"recovered_decisions"`
	RecoveredActions        uint32 `json:"recovered_actions"`
	RecoveredProgress       uint32 `json:"recovered_progress"`
	RecoveredPolicyOutcomes uint32 `json:"recovered_policy_outcomes"`
	AbandonedPolicyCalls    uint32 `json:"abandoned_policy_calls"`
	EnvironmentActions      uint32 `json:"environment_actions"`
}

type PausedInferenceCheckpoint struct {
	Schema                   string                    `json:"schema"`
	PublicRunAuthoritySHA256 string                    `json:"public_run_authority_sha256"`
	Episode                  cognition.EpisodeRef      `json:"episode"`
	PreCall                  SemanticPreCallCheckpoint `json:"pre_call"`
	Prefix                   RuntimePrefix             `json:"runtime_prefix"`
	SuccessfulActions        uint32                    `json:"successful_actions"`
}

func NewPausedInferenceCheckpoint(
	publicAuthoritySHA256 string,
	episode cognition.EpisodeRef,
	preCall SemanticPreCallCheckpoint,
	run cognitionruntime.RunResult,
	successfulActions uint32,
) (PausedInferenceCheckpoint, error) {
	checkpoint := PausedInferenceCheckpoint{
		Schema:                   PausedInferenceCheckpointSchemaV1,
		PublicRunAuthoritySHA256: publicAuthoritySHA256,
		Episode:                  episode, PreCall: preCall, Prefix: runtimePrefix(run),
		SuccessfulActions: successfulActions,
	}
	return checkpoint, checkpoint.Validate()
}

func (checkpoint PausedInferenceCheckpoint) Validate() error {
	if checkpoint.Schema != PausedInferenceCheckpointSchemaV1 ||
		!validDigest(checkpoint.PublicRunAuthoritySHA256) || checkpoint.Episode.Validate() != nil ||
		checkpoint.PreCall.Validate() != nil || checkpoint.PreCall.Bound.Attempt.Validate() != nil ||
		checkpoint.Prefix.Cycles == 0 || checkpoint.Prefix.PolicyCalls > checkpoint.Prefix.Cycles ||
		checkpoint.Prefix.EnvironmentActions > checkpoint.Prefix.Cycles ||
		checkpoint.SuccessfulActions == 0 || checkpoint.SuccessfulActions > checkpoint.Prefix.EnvironmentActions ||
		checkpoint.PreCall.Bound.Projection.SHA256 != checkpoint.PreCall.ProjectionRenderedSHA256 {
		return fmt.Errorf("paused cognition inference checkpoint is invalid")
	}
	return nil
}

func SealPausedInferenceCheckpoint(path string, checkpoint PausedInferenceCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, checkpoint, "paused cognition inference checkpoint")
}

func LoadPausedInferenceCheckpoint(path string) (PausedInferenceCheckpoint, error) {
	var checkpoint PausedInferenceCheckpoint
	if err := loadScenarioArtifact(path, &checkpoint, "paused cognition inference checkpoint"); err != nil {
		return PausedInferenceCheckpoint{}, err
	}
	if err := checkpoint.Validate(); err != nil {
		return PausedInferenceCheckpoint{}, err
	}
	return checkpoint, nil
}

func runtimePrefix(run cognitionruntime.RunResult) RuntimePrefix {
	return RuntimePrefix{
		Cycles: run.Cycles, PolicyCalls: run.PolicyCalls,
		RecoveredDecisions: run.RecoveredDecisions, RecoveredActions: run.RecoveredActions,
		RecoveredProgress:       run.RecoveredProgress,
		RecoveredPolicyOutcomes: run.RecoveredPolicyOutcomes,
		AbandonedPolicyCalls:    run.AbandonedPolicyCalls,
		EnvironmentActions:      run.EnvironmentActions,
	}
}

func runResultFromPrefix(prefix RuntimePrefix) cognitionruntime.RunResult {
	return cognitionruntime.RunResult{
		Cycles: prefix.Cycles, PolicyCalls: prefix.PolicyCalls,
		RecoveredDecisions: prefix.RecoveredDecisions, RecoveredActions: prefix.RecoveredActions,
		RecoveredProgress:       prefix.RecoveredProgress,
		RecoveredPolicyOutcomes: prefix.RecoveredPolicyOutcomes,
		AbandonedPolicyCalls:    prefix.AbandonedPolicyCalls,
		EnvironmentActions:      prefix.EnvironmentActions,
	}
}
