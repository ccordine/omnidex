package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/jackc/pgx/v5"
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

func requiredLifecycleWorkspaceQuery(request *http.Request) (string, string, error) {
	if err := validateExactQuery(request, "workspace_root", "workspace_identity"); err != nil {
		return "", "", err
	}
	if request == nil || request.URL == nil {
		return "", "", fmt.Errorf("lifecycle workspace query is unavailable")
	}
	root, rootPresent := oneQueryValue(request.URL.Query(), "workspace_root")
	identity, identityPresent := oneQueryValue(request.URL.Query(), "workspace_identity")
	if !rootPresent || !identityPresent {
		return "", "", fmt.Errorf(
			"workspace_root and workspace_identity must each occur exactly once",
		)
	}
	if err := model.ValidateChannelWorkspaceRoot(root); err != nil {
		return "", "", fmt.Errorf("workspace_root: %w", err)
	}
	if err := projectroot.ValidateDirectoryIdentity(identity); err != nil {
		return "", "", fmt.Errorf("workspace_identity: %w", err)
	}
	return root, identity, nil
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

func (s *Server) requireLifecycleWorkspaceIdentity(
	ctx context.Context,
	jobID int64,
	root string,
	identity string,
) (int, error) {
	if root == "" && identity == "" {
		required, err := s.repo.JobRequiresLifecycleWorkspaceAuthority(ctx, jobID)
		if errors.Is(err, pgx.ErrNoRows) {
			return http.StatusNotFound, err
		}
		if err != nil {
			return http.StatusInternalServerError, err
		}
		if required {
			return http.StatusBadRequest, fmt.Errorf(
				"lifecycle workspace_root and workspace_identity are required",
			)
		}
		return 0, nil
	}
	if root == "" || identity == "" {
		return http.StatusBadRequest, fmt.Errorf(
			"lifecycle workspace_root and workspace_identity must be supplied together",
		)
	}
	if err := s.requireServerWorkspaceIdentity(root, identity); err != nil {
		return http.StatusConflict, err
	}
	return 0, nil
}
