package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

type v3WorkspaceScope struct {
	Root     string
	Identity string
}

func codingWorkspaceForJob(job model.Job) (string, error) {
	scope, err := workspaceAuthorityForV3Job(job)
	if err != nil {
		return "", err
	}
	return scope.Root, nil
}

func workspaceAuthorityForV3Job(job model.Job) (v3WorkspaceScope, error) {
	if len(job.Metadata) == 0 {
		return v3WorkspaceScope{}, fmt.Errorf("workspace boundary requires job metadata")
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return v3WorkspaceScope{}, fmt.Errorf("decode workspace job metadata: %w", err)
	}
	raw, exists := metadata["client_cwd"]
	if !exists {
		return v3WorkspaceScope{}, fmt.Errorf("workspace boundary requires client_cwd")
	}
	var root string
	if err := json.Unmarshal(raw, &root); err != nil {
		return v3WorkspaceScope{}, fmt.Errorf("workspace boundary client_cwd must be a string: %w", err)
	}
	var identity string
	if rawIdentity, exists := metadata["client_workspace_identity"]; exists {
		if err := json.Unmarshal(rawIdentity, &identity); err != nil {
			return v3WorkspaceScope{}, fmt.Errorf(
				"workspace boundary client_workspace_identity must be a string: %w",
				err,
			)
		}
		if err := projectroot.ValidateDirectoryIdentity(identity); err != nil {
			return v3WorkspaceScope{}, fmt.Errorf("workspace boundary client identity: %w", err)
		}
	}
	return v3WorkspaceScope{Root: root, Identity: identity}, nil
}

func (s *Service) workspaceScopeForV3Job(job model.Job) (v3WorkspaceScope, error) {
	if s == nil {
		return v3WorkspaceScope{}, fmt.Errorf("workspace service is unavailable")
	}
	scope, err := workspaceAuthorityForV3Job(job)
	if err != nil {
		return v3WorkspaceScope{}, err
	}
	if err := s.requireHostWorkspaceRoot(scope.Root); err != nil {
		return v3WorkspaceScope{}, fmt.Errorf("bind job client_cwd %q: %w", scope.Root, err)
	}
	if scope.Identity != "" {
		currentIdentity, err := projectroot.DirectoryIdentity(scope.Root)
		if err != nil {
			return v3WorkspaceScope{}, fmt.Errorf(
				"attest job client_cwd %q identity: %w",
				scope.Root,
				err,
			)
		}
		if currentIdentity != scope.Identity {
			return v3WorkspaceScope{}, fmt.Errorf(
				"job client_cwd %q differs from its immutable client workspace identity",
				scope.Root,
			)
		}
	}
	return scope, nil
}

func (s *Service) requireWorkspaceScopeForV3Job(job model.Job, expectedRoot string) error {
	scope, err := s.workspaceScopeForV3Job(job)
	if err != nil {
		return err
	}
	if scope.Root != expectedRoot {
		return fmt.Errorf(
			"workspace root %q differs from exact job authority %q",
			expectedRoot,
			scope.Root,
		)
	}
	return nil
}

func (s *Service) requireHostWorkspaceRoot(root string) error {
	if s == nil {
		return fmt.Errorf("workspace service is unavailable")
	}
	return s.hostDirectoryAccess.ValidateWorkspaceRoot(root)
}
