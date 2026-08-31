package api

import (
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/projectroot"
)

func requiredWorkspaceIdentityQuery(request *http.Request) (string, error) {
	if request == nil || request.URL == nil {
		return "", fmt.Errorf("workspace identity query is unavailable")
	}
	values := request.URL.Query()["workspace_identity"]
	if len(values) != 1 {
		return "", fmt.Errorf("workspace_identity must occur exactly once")
	}
	if err := projectroot.ValidateDirectoryIdentity(values[0]); err != nil {
		return "", fmt.Errorf("workspace_identity: %w", err)
	}
	return values[0], nil
}

// requireServerWorkspaceIdentity proves that the server can reach the same
// physical directory the client independently attested. Equal path strings
// alone are never sufficient across a remote CORE_URL boundary.
func (s *Server) requireServerWorkspaceIdentity(
	exactRoot string,
	expectedIdentity string,
) error {
	if s == nil {
		return fmt.Errorf("server workspace identity authority is unavailable")
	}
	if err := projectroot.ValidateDirectoryIdentity(expectedIdentity); err != nil {
		return fmt.Errorf("workspace identity: %w", err)
	}
	if err := s.hostDirectoryAccess.ValidateWorkspaceRoot(exactRoot); err != nil {
		return fmt.Errorf("workspace root: %w", err)
	}
	actualIdentity, err := projectroot.DirectoryIdentity(exactRoot)
	if err != nil {
		return fmt.Errorf("attest server workspace identity: %w", err)
	}
	if actualIdentity != expectedIdentity {
		return fmt.Errorf(
			"server workspace identity differs from the exact client directory",
		)
	}
	return nil
}

func (s *Server) requireOptionalLifecycleWorkspaceIdentity(
	root string,
	identity string,
) error {
	if root == "" && identity == "" {
		return nil
	}
	if root == "" || identity == "" {
		return fmt.Errorf("lifecycle workspace_root and workspace_identity must be supplied together")
	}
	return s.requireServerWorkspaceIdentity(root, identity)
}
