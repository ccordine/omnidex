package cognitiongauntlet

import (
	"testing"
	"time"
)

func TestProductionSemanticReplayRequiresExactUTCMicrosecondTimes(t *testing.T) {
	started := time.Date(2026, time.August, 12, 1, 2, 3, 456_789_000, time.UTC)
	sealed := started.Add(time.Second)
	created := sealed
	if err := validateProductionSemanticReplayTimes(started, sealed, created); err != nil {
		t.Fatal(err)
	}

	offsetUTC := time.FixedZone("offset-zero", 0)
	mutations := map[string]func(*time.Time, *time.Time, *time.Time){
		"episode start offset": func(value, _, _ *time.Time) {
			*value = time.Date(2026, time.August, 12, 1, 2, 3, 456_789_000, offsetUTC)
		},
		"episode seal offset": func(_, value, _ *time.Time) {
			*value = value.In(offsetUTC)
		},
		"terminal seal offset": func(_, _, value *time.Time) {
			*value = value.In(offsetUTC)
		},
		"episode start fraction": func(value, _, _ *time.Time) {
			*value = value.Add(time.Nanosecond)
		},
		"episode seal fraction": func(_, value, _ *time.Time) {
			*value = value.Add(time.Nanosecond)
		},
		"terminal seal fraction": func(_, _, value *time.Time) {
			*value = value.Add(time.Nanosecond)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changedStart, changedSeal, changedCreated := started, sealed, created
			mutate(&changedStart, &changedSeal, &changedCreated)
			if validateProductionSemanticReplayTimes(
				changedStart, changedSeal, changedCreated,
			) == nil {
				t.Fatal("semantic replay accepted a noncanonical durable timestamp")
			}
		})
	}
}
