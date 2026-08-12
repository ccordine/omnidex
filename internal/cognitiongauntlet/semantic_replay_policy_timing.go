package cognitiongauntlet

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func validateSemanticPolicyTimingBounds(
	value queue.CognitionTracePolicyTiming,
	startedAt, sealedAt time.Time,
) error {
	if startedAt.IsZero() || sealedAt.Before(startedAt) ||
		value.StartedAt.Before(startedAt) || value.StartedAt.After(sealedAt) ||
		value.FinishedAt == nil || value.FinishedAt.After(sealedAt) {
		return fmt.Errorf("semantic policy timing lies outside the sealed episode")
	}
	return nil
}
