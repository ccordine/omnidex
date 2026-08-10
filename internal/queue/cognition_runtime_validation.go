package queue

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/model"
)

var cognitionDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func cognitionEpisodeIdentityValid(id cognition.EpisodeID) error {
	value := string(id)
	if value == "" || value != strings.TrimSpace(value) || len(value) > 256 ||
		!utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("cognition episode ID must be exact bounded UTF-8")
	}
	return nil
}

func validateCognitionEpisodeStart(command CognitionEpisodeStart) error {
	if err := validateStepAttemptAuthority(command.Authority); err != nil {
		return err
	}
	if err := cognitionEpisodeIdentityValid(command.EpisodeID); err != nil {
		return err
	}
	if err := command.AttestedBrain.Validate(); err != nil {
		return fmt.Errorf("cognition attested brain: %w", err)
	}
	if err := command.Scenario.Validate(); err != nil {
		return fmt.Errorf("cognition scenario: %w", err)
	}
	if err := command.Goal.Validate(); err != nil {
		return fmt.Errorf("cognition goal: %w", err)
	}
	if err := command.Completion.Validate(); err != nil {
		return fmt.Errorf("cognition completion authority: %w", err)
	}
	check, err := command.Completion.Resolve(command.Root.Desired)
	if err != nil {
		return fmt.Errorf("cognition root completion check is not code-authorized: %w", err)
	}
	if check != command.Root.CompletionCheck {
		return fmt.Errorf("cognition root completion check differs from the registered evaluator")
	}
	if err := command.ActionCatalog.Validate(); err != nil {
		return fmt.Errorf("cognition action catalog: %w", err)
	}
	if err := command.Budget.Validate(); err != nil {
		return fmt.Errorf("cognition runtime budget: %w", err)
	}
	if err := cognitionpolicy.ValidateRuntimeBudget(command.AttestedBrain.Ref, command.Budget); err != nil {
		return fmt.Errorf("cognition runtime budget brain binding: %w", err)
	}
	if command.Root.ParentID != "" || len(command.Root.DependsOn) != 0 {
		return fmt.Errorf("cognition root obligation cannot have a parent or dependency")
	}
	if _, err := cognition.NewObligationGraph(
		cognition.InitialObligationGeneration, command.Root.ID, []cognition.ObligationSpec{command.Root},
	); err != nil {
		return fmt.Errorf("cognition root obligation: %w", err)
	}
	wantID, err := cognition.DeriveObligationID(
		command.EpisodeID, cognition.InitialObligationGeneration, "", command.Root.Desired, command.Root.CompletionCheck,
	)
	if err != nil {
		return err
	}
	if command.Root.ID != wantID {
		return fmt.Errorf("%w: cognition obligation ID is not code-derived", ErrCognitionConflict)
	}
	if err := command.Transition.ValidateStart(); err != nil {
		return fmt.Errorf("cognition start transition: %w", err)
	}
	if command.Transition.Current.EpisodeID != command.EpisodeID {
		return fmt.Errorf("cognition start transition belongs to another episode")
	}
	return nil
}

func cognitionAuthorityMatches(authority model.StepAttemptAuthority, episode CognitionEpisode) error {
	if authority.JobID != episode.Authority.JobID || authority.Generation != episode.Authority.Generation ||
		authority.StepID != episode.Authority.StepID {
		return fmt.Errorf("%w: cognition actor targets another episode owner", ErrStaleStepAttempt)
	}
	return nil
}

func validCognitionTerminalStatus(status CognitionEpisodeStatus) bool {
	return status == CognitionEpisodeCompleted || status == CognitionEpisodeFailed || status == CognitionEpisodeCanceled
}
