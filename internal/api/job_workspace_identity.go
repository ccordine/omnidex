package api

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/projectroot"
)

func (s *Server) requireJobWorkspaceIdentity(ctx context.Context, jobID int64, root, identity string) error {
	authority, err := s.repo.LifecycleWorkspaceAuthority(ctx, jobID)
	if err != nil {
		return err
	}
	if !authority.Required || authority.Root != root || authority.Identity != identity {
		return fmt.Errorf("job differs from the exact workspace authority")
	}
	if authority.ClientOwned {
		return s.requireClientWorkspaceIdentity(ctx, root, identity)
	}
	if err := projectroot.ValidateDirectoryIdentity(identity); err != nil {
		return err
	}
	if err := s.hostDirectoryAccess.ValidateWorkspaceRoot(root); err != nil {
		return err
	}
	actual, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		return err
	}
	if actual != identity {
		return fmt.Errorf("server-local coding workspace directory changed")
	}
	return nil
}
