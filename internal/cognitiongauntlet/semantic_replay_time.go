package cognitiongauntlet

import (
	"fmt"
	"time"
)

func validateProductionSemanticReplayTimes(
	startedAt time.Time,
	sealedAt time.Time,
	terminalCreatedAt time.Time,
) error {
	if !isExactProductionReplayTime(startedAt) ||
		!isExactProductionReplayTime(sealedAt) ||
		!isExactProductionReplayTime(terminalCreatedAt) ||
		sealedAt.Before(startedAt) || !terminalCreatedAt.Equal(sealedAt) {
		return fmt.Errorf("production semantic replay timestamps are not exact UTC microseconds")
	}
	return nil
}

func isExactProductionReplayTime(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC && value.Nanosecond()%1_000 == 0
}
