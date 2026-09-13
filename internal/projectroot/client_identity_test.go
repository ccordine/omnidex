package projectroot

import (
	"strings"
	"testing"
)

func TestClientWorkspaceIdentityScopesLocalFilesystemFacts(t *testing.T) {
	first, err := ClientWorkspaceIdentity(strings.Repeat("1", 32), "directory_10_20")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ClientWorkspaceIdentity(strings.Repeat("2", 32), "directory_10_20")
	if err != nil || first == second {
		t.Fatalf("different clients share a workspace identity: %q, %q: %v", first, second, err)
	}
	clientID, directoryID, err := ParseClientWorkspaceIdentity(first)
	if err != nil || clientID != strings.Repeat("1", 32) || directoryID != "directory_10_20" {
		t.Fatalf("parse scoped identity: %q, %q: %v", clientID, directoryID, err)
	}
	for _, invalid := range []string{
		"directory_10_20", "", first + "_extra", strings.ToUpper(first),
		"client_" + strings.Repeat("1", 32) + "_directory_10_0",
	} {
		if err := ValidateClientWorkspaceIdentity(invalid); err == nil {
			t.Errorf("accepted invalid client workspace identity %q", invalid)
		}
	}
}
