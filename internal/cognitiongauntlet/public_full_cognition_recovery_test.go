package cognitiongauntlet

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPublicRuntimeCounterMismatchRequiresRegisteredLiveRecovery(t *testing.T) {
	resources := Resources{PolicyCallsConsumed: 3, ModelCalls: 3, EnvironmentActions: 2}
	run := cognitionruntime.RunResult{PolicyCalls: 1, EnvironmentActions: 1}
	if err := validatePublicRuntimeCounters(resources, run, ""); err == nil {
		t.Fatal("ordinary runtime counter mismatch was accepted")
	}
	for _, port := range liveStalePorts() {
		if err := validatePublicRuntimeCounters(resources, run, port); err != nil {
			t.Fatalf("registered recovery port %q: %v", port, err)
		}
	}
	if err := validatePublicRuntimeCounters(resources, run, "unregistered"); err == nil {
		t.Fatal("unregistered recovery port was accepted")
	}
}
