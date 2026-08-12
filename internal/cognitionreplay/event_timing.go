package cognitionreplay

import (
	"fmt"
	"time"
)

func validateEventTiming(
	timing EventTiming,
	eventSources []SourceRef,
	startedAt, sealedAt time.Time,
) error {
	if timing.Timestamp.IsZero() || timing.Timestamp.Before(startedAt) ||
		timing.Timestamp.After(sealedAt) || timing.ElapsedNanoseconds < 0 ||
		timing.Timestamp.Sub(startedAt).Nanoseconds() != timing.ElapsedNanoseconds ||
		timing.Source.Validate() != nil {
		return fmt.Errorf("replay event timing is not exact and episode-bound")
	}
	for _, source := range eventSources {
		if source == timing.Source {
			return nil
		}
	}
	return fmt.Errorf("replay event timing does not cite one exact event source")
}
