package model

import "testing"

func TestValidateChannelWorkspaceRootRequiresExactAbsoluteCanonicalValue(t *testing.T) {
	t.Parallel()
	if err := ValidateChannelWorkspaceRoot("/srv/workspaces/default"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		"", "relative/path", " /srv/workspaces/default", "/srv/workspaces/default ", "/srv/workspaces/../other",
	} {
		if err := ValidateChannelWorkspaceRoot(invalid); err == nil {
			t.Errorf("invalid workspace root passed: %q", invalid)
		}
	}
}
