package queue

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
)

func validateEnvironmentStart(
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
	start cognition.Transition,
) error {
	if err := episode.Validate(); err != nil {
		return err
	}
	if err := scenario.Validate(); err != nil {
		return err
	}
	if err := start.ValidateStart(); err != nil {
		return err
	}
	if start.Current.EpisodeID != episode.ID {
		return fmt.Errorf("%w: environment start belongs to another episode", cognition.ErrInvalidTransition)
	}
	return nil
}

func environmentActionAuthority(action cognition.RegisteredAction) model.StepAttemptAuthority {
	return model.StepAttemptAuthority{
		JobID: action.Actor.JobID, Generation: action.Actor.Generation,
		StepID: action.Actor.StepID, Attempt: int64(action.Actor.Attempt),
		WorkerID: action.Actor.WorkerID,
	}
}

func validateEnvironmentActionIdentity(
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) error {
	if err := episode.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil || expected.EpisodeID != episode.ID {
		return fmt.Errorf("%w: environment action has invalid expected revision", cognition.ErrInvalidRevision)
	}
	if action.ID == "" || action.Actor.Validate() != nil || action.Actor.Attempt > math.MaxInt64 ||
		action.Schema.Validate() != nil ||
		action.Request.Validate() != nil {
		return fmt.Errorf("%w: environment action identity is invalid", cognition.ErrInvalidAction)
	}
	seen := make(map[string]struct{}, len(action.EvidenceRefs))
	for _, ref := range action.EvidenceRefs {
		if err := ref.Validate(); err != nil || ref.Revision.EpisodeID != episode.ID {
			return fmt.Errorf("%w: environment action evidence is invalid", cognition.ErrInvalidEvidence)
		}
		raw, _, err := cognitionJSON(ref)
		if err != nil {
			return err
		}
		key := string(raw)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%w: environment action evidence is duplicated", cognition.ErrInvalidEvidence)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func decodeExactEnvironmentTransition(raw, sha string) (cognition.Transition, error) {
	var value cognition.Transition
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return cognition.Transition{}, fmt.Errorf("decode environment transition: %w", err)
	}
	wantRaw, wantSHA, err := cognitionJSON(value)
	if err != nil || string(wantRaw) != raw || wantSHA != sha {
		return cognition.Transition{}, fmt.Errorf("%w: persisted environment transition is not exact", cognition.ErrEnvironmentJournalConflict)
	}
	return value, nil
}

func decodeExactEnvironmentReceipt(
	episode cognition.EpisodeRef,
	raw, sha string,
) (cognition.EnvironmentReceipt, error) {
	var value cognition.EnvironmentReceipt
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return cognition.EnvironmentReceipt{}, fmt.Errorf("decode environment receipt: %w", err)
	}
	wantRaw, wantSHA, err := cognitionJSON(value)
	if err != nil || string(wantRaw) != raw || wantSHA != sha {
		return cognition.EnvironmentReceipt{}, fmt.Errorf("%w: persisted environment receipt is not exact", cognition.ErrEnvironmentJournalConflict)
	}
	if err := value.Validate(episode); err != nil {
		return cognition.EnvironmentReceipt{}, fmt.Errorf("persisted environment receipt: %w", err)
	}
	return value, nil
}
