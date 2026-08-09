package worker

import (
	"fmt"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func (session *directCodingSession) requireCurrentRepositoryAuthority(phase string) error {
	if session == nil || session.runtime == nil || session.runtime.ctx == nil || session.repositoryIndex == nil {
		return fmt.Errorf("repository freshness check requires one active indexed session")
	}
	phase = strings.TrimSpace(phase)
	if phase == "" {
		return fmt.Errorf("repository freshness check requires one phase")
	}
	current, err := repositoryfacts.BuildGitSnapshot(
		session.runtime.ctx, session.root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		return fmt.Errorf("verify repository freshness before %s: %w", phase, err)
	}
	expected := session.repositoryIndex.Snapshot
	if current.ID != expected.ID {
		return fmt.Errorf(
			"stale_repository_authority: repository changed before %s; indexed=%s current=%s",
			phase, expected.ID, current.ID,
		)
	}
	return nil
}
