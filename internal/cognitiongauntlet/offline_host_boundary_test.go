package cognitiongauntlet

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestHostProcessAuthorityContainsNoOracleOrEvaluatorCredential(t *testing.T) {
	t.Parallel()
	config := hostProcessConfig{
		Schema: hostProcessConfigSchemaV1, DatabaseURL: "postgres://host:secret@127.0.0.1/db",
		DatabaseSchema: "runtime", HostSchema: "host", ExpectedRole: "restricted-host",
		Scenario:         cognition.ScenarioRef{ID: "scenario", SHA256: strings.Repeat("a", 64)},
		HostScenarioPath: "/private/host.json", PublicBundlePath: "/public/bootstrap.json",
		ReadyPath: "/private/ready.json", EnvironmentToken: "environment-token",
		ExecutableSHA256: strings.Repeat("b", 64), SourceSHA256: strings.Repeat("c", 64),
		OmnidexCommit: strings.Repeat("d", 40),
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"oracle", "evaluator", "witness", "seed", "private_oracle", "credential",
	} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("host process authority exposes forbidden label %q: %s", forbidden, raw)
		}
	}
}

func TestHostReceiptRequiresExactProcessChronology(t *testing.T) {
	t.Parallel()
	base := time.Now().UTC()
	receipt := OfflineHostReceipt{
		Schema: offlineHostReceiptSchemaV1, PID: 21, Role: "restricted-host",
		Scenario:     cognition.ScenarioRef{ID: "scenario", SHA256: strings.Repeat("a", 64)},
		ConfigSHA256: strings.Repeat("b", 64), ReadySHA256: strings.Repeat("c", 64),
		StartedAt: base.Add(time.Second), ExitedAt: base.Add(3 * time.Second),
	}
	if err := receipt.validateChronology(
		base, base.Add(2*time.Second), base.Add(4*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	receipt.ExitedAt = base.Add(1500 * time.Millisecond)
	if err := receipt.validateChronology(
		base, base.Add(2*time.Second), base.Add(4*time.Second),
	); err == nil {
		t.Fatal("host receipt accepted exit before inference stopped")
	}
}
