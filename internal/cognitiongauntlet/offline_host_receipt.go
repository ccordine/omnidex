package cognitiongauntlet

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

const offlineHostReceiptSchemaV1 = "omnidex.offline-cognition-host-receipt.v1"

type OfflineHostReceipt struct {
	Schema       string                `json:"schema"`
	PID          int                   `json:"pid"`
	Role         string                `json:"role"`
	Scenario     cognition.ScenarioRef `json:"scenario"`
	ConfigSHA256 string                `json:"config_sha256"`
	ReadySHA256  string                `json:"ready_sha256"`
	StartedAt    time.Time             `json:"started_at"`
	ExitedAt     time.Time             `json:"exited_at"`
}

func (receipt OfflineHostReceipt) Validate() error {
	if receipt.Schema != offlineHostReceiptSchemaV1 || receipt.PID <= 0 ||
		receipt.Scenario.Validate() != nil || !validDigest(receipt.ConfigSHA256) ||
		!validDigest(receipt.ReadySHA256) || receipt.StartedAt.IsZero() ||
		receipt.ExitedAt.Before(receipt.StartedAt) ||
		requireExact(receipt.Role, "offline host receipt role", 256) != nil {
		return fmt.Errorf("offline cognition host receipt is invalid")
	}
	return nil
}

func (receipt OfflineHostReceipt) validateChronology(
	generatorExited time.Time,
	inferenceExited time.Time,
	evaluatorStarted time.Time,
) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if receipt.StartedAt.Before(generatorExited) || inferenceExited.Before(receipt.StartedAt) ||
		receipt.ExitedAt.Before(inferenceExited) || evaluatorStarted.Before(receipt.ExitedAt) {
		return fmt.Errorf("offline cognition host chronology is invalid")
	}
	return nil
}
