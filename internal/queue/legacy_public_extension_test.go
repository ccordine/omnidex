package queue

import "testing"

func TestLegacyPublicExtensionDescriptorMatchesFrozenAudit(t *testing.T) {
	values := []legacyExtension{
		{Name: "pg_trgm", Version: "1.6", Owner: "agent", Relocatable: true},
		{Name: "pgcrypto", Version: "1.3", Owner: "agent", Relocatable: true},
		{Name: "vector", Version: "0.8.2", Owner: "agent", Relocatable: true},
	}
	if got := legacyExtensionDescriptorSHA256(values); got != legacyExpectedExtensionSHA256 {
		t.Fatalf("legacy extension descriptor sha256=%s want %s", got, legacyExpectedExtensionSHA256)
	}
}
