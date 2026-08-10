package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

func collectReplayObservations(
	known map[cognition.ObservationID]cognition.Observation,
	transition cognition.Transition,
) error {
	for _, observation := range transition.Observations {
		if _, duplicate := known[observation.ID]; duplicate {
			return fmt.Errorf("private replay produced duplicate observation %q", observation.ID)
		}
		known[observation.ID] = observation
	}
	return nil
}

func verifyReplayObservations(
	episode SealedEpisode,
	expected map[cognition.ObservationID]cognition.Observation,
) error {
	seen := make(map[cognition.ObservationID]struct{}, len(expected))
	for _, entry := range episode.Manifest.Trace {
		if entry.Kind != TraceObservation {
			continue
		}
		var observation cognition.Observation
		if err := decodeTracePayload(entry.Payload, &observation, "private replay observation"); err != nil {
			return err
		}
		if err := observation.Validate(); err != nil || entry.Revision == nil ||
			observation.Revision != *entry.Revision || string(observation.ID) != entry.ID {
			return fmt.Errorf("sealed observation trace authority is invalid")
		}
		want, exists := expected[observation.ID]
		if !exists || !reflect.DeepEqual(observation, want) {
			return fmt.Errorf("sealed observation %q differs from private replay", observation.ID)
		}
		if _, duplicate := seen[observation.ID]; duplicate {
			return fmt.Errorf("sealed observation %q is duplicated", observation.ID)
		}
		seen[observation.ID] = struct{}{}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf(
			"sealed observation trace has %d records; private replay produced %d",
			len(seen), len(expected),
		)
	}
	return nil
}
