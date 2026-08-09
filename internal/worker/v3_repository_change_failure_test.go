package worker

import "testing"

func TestRepositoryGoPathFreeDiagnosticRejectsBareFileNames(t *testing.T) {
	t.Parallel()
	for _, diagnostic := range []string{
		"first.go has wrong value",
		`"first.go" has wrong value`,
		"(first_test.go): wrong value",
		"first.go, wrong value",
		"wrong value [first.go]",
		"config.yaml has wrong value",
		`wrong value in "go.mod"`,
		"hidden .env is invalid",
		"CONFIG.YAML has wrong value",
		"First.GO has wrong value",
		"hidden .ENV is invalid",
	} {
		if accepted, ok := validateRepositoryGoPathFreeDiagnostic(diagnostic); ok {
			t.Fatalf("accepted bare Go basename diagnostic %q as %q", diagnostic, accepted)
		}
	}
}

func TestRepositoryGoPathFreeDiagnosticPreservesLegitimateText(t *testing.T) {
	t.Parallel()
	for _, diagnostic := range []string{
		"got 11, want 1",
		"cannot use duration as int",
		"operation should keep going",
		"method Go returned the wrong value",
		"got version 1.24, want 1.25",
	} {
		accepted, ok := validateRepositoryGoPathFreeDiagnostic(diagnostic)
		if !ok || accepted != diagnostic {
			t.Fatalf("legitimate diagnostic %q rejected as %q", diagnostic, accepted)
		}
	}
}
