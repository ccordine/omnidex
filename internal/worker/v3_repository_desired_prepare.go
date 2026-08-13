package worker

import (
	"context"
	"fmt"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

// prepareVerifiedDesiredRepositoryState stages code-derived physical
// transitions behind a desired-graph authority. Verification failure is
// terminal: no model receives paths, commands, or responsibility for choosing
// a different physical transition.
func (session *directCodingSession) prepareVerifiedDesiredRepositoryState(
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	graph repositoryfacts.DesiredArtifactGraph,
	desired []changeapply.DesiredFileState,
	commands []testCommand,
) (*verifiedRepositoryChangeStage, error) {
	if session == nil || session.runtime == nil || session.runtime.ctx == nil {
		return nil, fmt.Errorf("prepare desired repository state requires one active coding session")
	}
	if err := graph.Validate(snapshot, analysis); err != nil {
		return nil, fmt.Errorf("prepare desired repository graph: %w", err)
	}
	stage, err := changeapply.PlanFileStateTransitions(
		session.runtime.ctx,
		changeapply.FileStateInput{
			Snapshot: snapshot, Analysis: analysis, OwnerID: graph.ID,
			Desired: append([]changeapply.DesiredFileState(nil), desired...),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("stage desired repository state: %w", err)
	}
	authority, err := newRepositoryVerificationAuthority(
		snapshot.ID, graph.ID, commands, stage,
	)
	if err != nil {
		return nil, cleanupFailedRepositoryStage(stage, err)
	}
	if err := session.runExistingRepositoryVerification(
		stage.Workspace(), repositoryVerificationStaged,
		commands, authority, nil,
		func(ctx context.Context) error { return stage.VerifyExactWorkspace(ctx) },
	); err != nil {
		return nil, cleanupFailedRepositoryStage(stage, err)
	}
	prepared, err := newVerifiedRepositoryChangeStage(graph.ID, commands, stage)
	if err != nil {
		return nil, cleanupFailedRepositoryStage(stage, err)
	}
	return prepared, nil
}
