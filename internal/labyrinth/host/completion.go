package host

import (
	"context"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/jackc/pgx/v5"
)

var _ cognitionruntime.CompletionEvaluator = (*Environment)(nil)

// Evaluate reconstructs exact committed world truth and evaluates only the
// obligation's registered predicate. No facts or oracle data leave the host.
func (environment *Environment) Evaluate(
	ctx context.Context,
	request cognitionruntime.CompletionRequest,
) (cognition.CompletionResult, error) {
	if err := environment.validate(ctx); err != nil {
		return cognition.CompletionResult{}, err
	}
	if err := request.Binding.Validate(); err != nil {
		return cognition.CompletionResult{}, err
	}
	if err := environment.authorize(ctx, request.Binding.Attempt); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return cognition.CompletionResult{}, contextErr
		}
		return cognition.CompletionResult{}, fmt.Errorf("%w: completion actor is stale", cognition.ErrAuthorityDenied)
	}
	if request.Binding.Episode != environment.episode ||
		request.Revision.EpisodeID != environment.episode.ID {
		return cognition.CompletionResult{}, fmt.Errorf("%w: completion belongs to another episode", cognition.ErrInvalidRevision)
	}
	if err := validateCompletionRequest(request); err != nil {
		return cognition.CompletionResult{}, err
	}

	tx, err := environment.store.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	defer tx.Rollback(ctx)
	stored, err := loadEpisodeRow(ctx, tx, environment.store.schema, environment.episode, false)
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	if stored.Current != request.Revision {
		return cognition.CompletionResult{}, fmt.Errorf("%w: completion revision is stale", cognition.ErrInvalidRevision)
	}
	scenario, err := environment.resolveScenario(ctx, stored.Scenario)
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	if !reflect.DeepEqual(request.Goal, scenario.Goal()) {
		return cognition.CompletionResult{}, fmt.Errorf("%w: completion root goal changed", cognition.ErrInvalidCompletionCheck)
	}
	expectedCheck, err := labyrinth.NewCompletionCheck()
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	if request.Obligation.CompletionCheck != expectedCheck {
		return cognition.CompletionResult{}, fmt.Errorf("%w: completion evaluator is not registered", cognition.ErrInvalidCompletionCheck)
	}
	if request.EnvironmentTerminal != stored.Terminal ||
		request.PublicOutcome != completionPublicOutcome(stored.Terminal) {
		return cognition.CompletionResult{}, fmt.Errorf("%w: completion terminal authority changed", cognition.ErrInvalidCompletionResult)
	}
	history, err := loadSuccessfulHistory(ctx, tx, environment.store.schema, environment.episode)
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	candidate, err := reconstructCandidate(ctx, environment, scenario, stored, history)
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	satisfied, err := candidate.EvaluateGoal(
		ctx, environment.episode, request.Revision, request.Obligation.Desired,
	)
	if closeErr := candidate.Close(); closeErr != nil {
		return cognition.CompletionResult{}, fmt.Errorf("close Labyrinth completion kernel: %w", closeErr)
	}
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	if reflect.DeepEqual(request.Obligation.Desired, request.Goal) && satisfied != stored.Terminal {
		return cognition.CompletionResult{}, fmt.Errorf("%w: reconstructed root completion differs from durable terminal state", ErrReceiptCorrupt)
	}
	evidence, err := completionEvidence(request, stored, history, satisfied)
	if err != nil {
		return cognition.CompletionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return cognition.CompletionResult{}, fmt.Errorf("commit Labyrinth completion read: %w", err)
	}
	return completionResult(request, satisfied, evidence)
}

func validateCompletionRequest(request cognitionruntime.CompletionRequest) error {
	if err := request.Binding.Validate(); err != nil {
		return err
	}
	if !completionDigest(request.SnapshotSHA256) {
		return fmt.Errorf("%w: completion snapshot hash is invalid", cognition.ErrInvalidCompletionResult)
	}
	if err := request.Goal.Validate(); err != nil {
		return err
	}
	if err := request.Revision.Validate(); err != nil {
		return err
	}
	if err := request.Obligation.Validate(); err != nil {
		return err
	}
	if request.Obligation.Status != cognition.ObligationActive {
		return fmt.Errorf("%w: completion obligation is not active in the bound generation", cognition.ErrInvalidCompletionResult)
	}
	if len(request.EvidenceRefs) > cognition.MaxEvidenceRefs {
		return fmt.Errorf("%w: completion evidence packet is too large", cognition.ErrInvalidEvidence)
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(request.EvidenceRefs))
	for index, ref := range request.EvidenceRefs {
		if err := ref.Validate(); err != nil || ref.Revision.EpisodeID != request.Binding.Episode.ID ||
			ref.Revision.Number > request.Revision.Number {
			return fmt.Errorf("%w: completion evidence %d is invalid", cognition.ErrInvalidEvidence, index)
		}
		if _, duplicate := available[ref]; duplicate {
			return fmt.Errorf("%w: completion evidence %d is duplicated", cognition.ErrInvalidEvidence, index)
		}
		available[ref] = struct{}{}
	}
	for index, ref := range request.Obligation.SupportingRefs {
		if _, exists := available[ref]; !exists {
			return fmt.Errorf("%w: obligation evidence %d is absent", cognition.ErrInvalidEvidence, index)
		}
	}
	return nil
}

func completionResult(
	request cognitionruntime.CompletionRequest,
	satisfied bool,
	evidence []cognition.EvidenceRef,
) (cognition.CompletionResult, error) {
	outcome := cognition.CompletionUnsatisfied
	if satisfied {
		outcome = cognition.CompletionSatisfied
	} else if len(evidence) != 0 {
		return cognition.CompletionResult{}, fmt.Errorf(
			"%w: unsatisfied completion cannot carry evidence", cognition.ErrInvalidCompletionResult,
		)
	}
	return cognition.NewCompletionResult(
		request.Obligation.ID, request.Obligation.CompletionCheck, request.Revision, outcome, evidence,
	)
}

func completionPublicOutcome(terminal bool) string {
	if terminal {
		return labyrinth.PublicOutcomeGoalSatisfied
	}
	return ""
}

func completionDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
