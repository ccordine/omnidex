package modelcontext

import "testing"

func TestContainsPathIdentityRejectsQualifiedAndBareIdentities(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"/tmp/private/value", `C:\private\value`, "nested/private/value",
		"first.go has wrong value", "CONFIG.YAML is invalid", "go.mod", ".ENV",
	} {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if !ContainsPathIdentity(value) {
				t.Fatalf("path identity %q was accepted", value)
			}
		})
	}
}

func TestContainsPathIdentityAcceptsPathFreeDiagnostics(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"expected 3 but received 2", "version 1.25 remains valid", "registered action failed",
	} {
		if ContainsPathIdentity(value) {
			t.Fatalf("path-free text %q was rejected", value)
		}
	}
}
