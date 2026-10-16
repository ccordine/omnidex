package model

import "testing"

func TestTaskFileReferencesDescribeHostContextNotAccessGrants(t *testing.T) {
	for _, value := range []string{"/etc/service/config", "../deployment/compose.yaml", ".env", ".git/config", "bin/tool", "node_modules/package/index.js", "server:/srv/app/config", `C:\work\app\config`} {
		if err := ValidateTaskFileReference(value); err != nil {
			t.Fatalf("context path %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "  ", "bad\x00path", "bad\npath"} {
		if err := ValidateTaskFileReference(value); err == nil {
			t.Fatalf("invalid context path %q accepted", value)
		}
	}
}
