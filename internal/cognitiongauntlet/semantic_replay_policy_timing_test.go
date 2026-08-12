package cognitiongauntlet

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticPolicyTimingIsBoundedBySealedEpisode(t *testing.T) {
	started := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	sealed := started.Add(time.Minute)
	finished := started.Add(time.Second)
	value := queue.CognitionTracePolicyTiming{
		Schema: queue.CognitionTracePolicyTimingSchemaV1,
		CallID: "call-one", Status: cognitionpolicy.CallResultAccepted,
		StartedAt: started, FinishedAt: &finished,
	}
	if err := validateSemanticPolicyTimingBounds(value, started, sealed); err != nil {
		t.Fatal(err)
	}

	before := started.Add(-time.Microsecond)
	after := sealed.Add(time.Microsecond)
	for name, mutate := range map[string]func(*queue.CognitionTracePolicyTiming){
		"started_before_episode": func(value *queue.CognitionTracePolicyTiming) {
			value.StartedAt = before
		},
		"started_after_seal": func(value *queue.CognitionTracePolicyTiming) {
			value.StartedAt = after
			value.FinishedAt = &after
		},
		"finished_after_seal": func(value *queue.CognitionTracePolicyTiming) {
			value.FinishedAt = &after
		},
		"missing_finish": func(value *queue.CognitionTracePolicyTiming) {
			value.FinishedAt = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := value
			mutate(&changed)
			if validateSemanticPolicyTimingBounds(changed, started, sealed) == nil {
				t.Fatal("semantic replay accepted policy timing outside the episode")
			}
		})
	}
}
