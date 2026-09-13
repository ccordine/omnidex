package api

import (
	"os"
	"strings"
	"testing"
)

func TestCLIWorkspaceAuthorityCannotUseServerFilesystem(t *testing.T) {
	for _, name := range []string{
		"workspace_identity_authority.go", "cli_workspace_connection.go",
		"cli_chat_session.go", "channel_session.go", "channel_session_state.go", "channel_session_turn.go",
	} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"hostDirectoryAccess", "projectroot.DirectoryIdentity(", "requireServerWorkspaceIdentity", "os.Stat(", "os.Lstat("} {
			if strings.Contains(string(data), forbidden) {
				t.Errorf("CLI workspace boundary %s contains server filesystem operation %q", name, forbidden)
			}
		}
	}
}
