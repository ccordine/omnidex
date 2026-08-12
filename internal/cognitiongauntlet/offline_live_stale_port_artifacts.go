package cognitiongauntlet

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

const (
	liveStalePortCheckpointSchemaV1 = "omnidex.live-stale-port-checkpoint.v1"
	liveStalePortRejectionSchemaV2  = "omnidex.live-stale-port-rejection.v2"
)

type liveStalePortCheckpoint struct {
	Schema        string                     `json:"schema"`
	Port          liveStalePort              `json:"port"`
	PID           int                        `json:"pid"`
	Attempt       model.StepAttemptAuthority `json:"attempt"`
	CommandSHA256 string                     `json:"command_sha256"`
	EnteredAt     time.Time                  `json:"entered_at"`
}

type liveStalePortRejection struct {
	Schema                     string                         `json:"schema"`
	Port                       liveStalePort                  `json:"port"`
	PID                        int                            `json:"pid"`
	Attempt                    model.StepAttemptAuthority     `json:"attempt"`
	CommandSHA256              string                         `json:"command_sha256"`
	ErrorClass                 string                         `json:"error_class"`
	ProviderRequestDisposition llm.ProviderRequestDisposition `json:"provider_request_disposition"`
	ProviderUsagePresent       bool                           `json:"provider_usage_present"`
	ProviderUsage              llm.ProviderGenerationUsage    `json:"provider_usage"`
	ProviderDoneReason         string                         `json:"provider_done_reason"`
	RejectedAt                 time.Time                      `json:"rejected_at"`
}

func (checkpoint liveStalePortCheckpoint) Validate() error {
	if checkpoint.Schema != liveStalePortCheckpointSchemaV1 ||
		checkpoint.Port.Validate() != nil || checkpoint.PID <= 0 ||
		!validTakeoverAttempt(checkpoint.Attempt) ||
		!validDigest(checkpoint.CommandSHA256) || checkpoint.EnteredAt.IsZero() {
		return fmt.Errorf("live stale-port checkpoint is invalid")
	}
	return nil
}

func (rejection liveStalePortRejection) Validate() error {
	if rejection.Schema != liveStalePortRejectionSchemaV2 ||
		rejection.Port.Validate() != nil || rejection.PID <= 0 ||
		!validTakeoverAttempt(rejection.Attempt) ||
		!validDigest(rejection.CommandSHA256) || rejection.RejectedAt.IsZero() ||
		rejection.ErrorClass != rejection.Port.expectedError() {
		return fmt.Errorf("live stale-port rejection is invalid")
	}
	if rejection.Port == liveStalePolicyFinish {
		if err := rejection.validateProviderRequestEvidence(); err != nil {
			return fmt.Errorf("live stale policy rejection lacks exact provider request evidence")
		}
	} else if rejection.ProviderRequestDisposition != llm.ProviderRequestNotDispatched ||
		rejection.ProviderUsagePresent ||
		rejection.ProviderUsage != (llm.ProviderGenerationUsage{}) ||
		rejection.ProviderDoneReason != "" {
		return fmt.Errorf("nonpolicy stale rejection carries provider evidence")
	}
	return nil
}

func (rejection liveStalePortRejection) validateProviderRequestEvidence() error {
	switch rejection.ProviderRequestDisposition {
	case llm.ProviderRequestDispatched:
		if rejection.ProviderDoneReason == "" ||
			(rejection.ProviderUsagePresent && rejection.ProviderUsage.ValidateSuccessful() != nil) {
			return fmt.Errorf("dispatched provider evidence is invalid")
		}
	case llm.ProviderRequestWriteIndeterminate:
		if rejection.ProviderDoneReason != "" || rejection.ProviderUsagePresent ||
			rejection.ProviderUsage != (llm.ProviderGenerationUsage{}) {
			return fmt.Errorf("indeterminate provider write claims response evidence")
		}
	default:
		return fmt.Errorf("provider request did not reach the provider")
	}
	return nil
}

func (rejection liveStalePortRejection) ValidateFor(
	checkpoint liveStalePortCheckpoint,
) error {
	if err := rejection.Validate(); err != nil {
		return err
	}
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	if rejection.Port != checkpoint.Port || rejection.PID != checkpoint.PID ||
		rejection.Attempt != checkpoint.Attempt ||
		rejection.CommandSHA256 != checkpoint.CommandSHA256 ||
		rejection.RejectedAt.Before(checkpoint.EnteredAt) {
		return fmt.Errorf("live stale-port rejection changed its paused command authority")
	}
	return nil
}

func loadLiveStalePortCheckpoint(path string) (liveStalePortCheckpoint, error) {
	var checkpoint liveStalePortCheckpoint
	if err := loadStrictJSONFile(path, &checkpoint, "live stale-port checkpoint"); err != nil {
		return liveStalePortCheckpoint{}, err
	}
	return checkpoint, checkpoint.Validate()
}

func loadLiveStalePortRejection(path string) (liveStalePortRejection, error) {
	var rejection liveStalePortRejection
	if err := loadStrictJSONFile(path, &rejection, "live stale-port rejection"); err != nil {
		return liveStalePortRejection{}, err
	}
	return rejection, rejection.Validate()
}
