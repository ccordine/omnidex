package cognitiongauntlet

import (
	"os"
	"strings"
	"testing"
)

func TestRuntimeActivationUsesOneJournaledProviderProcessAuthority(t *testing.T) {
	source := ""
	for _, name := range []string{"full_cognition_runtime.go", "provider_identity_outcomes.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		source += string(raw)
	}
	if strings.Count(source, "cognitionpolicy.ObserveProviderProcess(") != 1 ||
		strings.Contains(source, "RecordProviderProcessObservation(") ||
		strings.Count(source, "store.RecordProviderProcessFailure(") != 1 {
		t.Fatal("runtime activation must be persisted atomically by cognition episode start")
	}
	if strings.Contains(source, "cognitionpolicy.AttestLocalHostHardware(") {
		t.Fatal("runtime activation retains a redundant unjournaled host observation")
	}
	if !strings.Contains(source, "components.brainBootstrap,") ||
		!strings.Contains(source, "store.RecordProviderProcessFailure(ctx, bootstrap,") {
		t.Fatal("provider process failure does not retain the indivisible successful bootstrap evidence")
	}
	for _, name := range []string{
		"full_cognition_run.go",
		"public_full_cognition_prepare.go",
		"extended_runtime_run.go",
		"full_cognition_execute.go",
	} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "observeRuntimeProviderActivation(") {
			t.Fatalf("%s does not obtain the exact activation before episode start", name)
		}
	}
	for _, name := range []string{
		"full_cognition_run.go",
		"public_full_cognition_start.go",
		"extended_runtime_start.go",
	} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "ProviderProcessActivation: activation") {
			t.Fatalf("%s does not bind provider activation into atomic episode start", name)
		}
	}
}
