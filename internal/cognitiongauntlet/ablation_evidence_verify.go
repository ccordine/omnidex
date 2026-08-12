package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func verifyAblationEvidenceArtifact(artifact ablationEvidenceArtifact) error {
	root := artifact.Root
	class, private, err := ablationReplayClass(root.Variant)
	publicSHA, publicErr := root.PublicRunAuthority.SHA256()
	episode, episodeErr := PublicVariantEpisodeRef(root.PublicRunAuthority)
	if artifact.Schema != AblationEvidenceSchemaV1 || root.Schema != AblationEvidenceSchemaV1 ||
		err != nil || publicErr != nil || episodeErr != nil || root.Class != class ||
		root.PublicRunAuthority.Variant != root.Variant ||
		root.PublicRunAuthoritySHA256 != publicSHA || episode.ID != root.EpisodeID ||
		(cognition.EpisodeRef{ID: root.EpisodeID}).Validate() != nil ||
		root.BrainBootstrap.Validate() != nil || root.ProviderActivation.Validate() != nil ||
		root.WorldCatalog.Validate() != nil ||
		root.WorldCatalog.SHA256 != root.PublicRunAuthority.ActionCatalogSHA256 ||
		root.TerminalCause.Validate() != nil || root.Terminal.Validate() != nil ||
		root.ChunkedBlobs == nil || artifact.Blobs == nil {
		return fmt.Errorf("ablation evidence root authority is invalid: %v", err)
	}
	if root.Terminal.Revision.EpisodeID != root.EpisodeID {
		return fmt.Errorf("ablation evidence terminal belongs to another episode")
	}
	if err := verifyAblationRootTaskAuthority(root); err != nil {
		return err
	}
	if err := verifyAblationTransitions(root); err != nil {
		return err
	}
	if err := verifyAblationLedgerEvidence(root.Variant, root.Ledger); err != nil {
		return err
	}
	if err := verifyAblationWorkingSetEvidence(root.Variant, root.WorkingSet); err != nil {
		return err
	}
	role := cognitionreplay.ChunkedBlobPublicAgentKnowledge
	if private {
		role = cognitionreplay.ChunkedBlobPrivateWorld
	}
	store, err := newAblationEvidenceContentStore(artifact)
	if err != nil {
		return err
	}
	if err := verifyAblationCalls(root.Calls, store, role); err != nil {
		return err
	}
	if err := verifyAblationCallInputAuthorities(root); err != nil {
		return err
	}
	if err := verifyAblationContextBudgetEvidence(root, store, role); err != nil {
		return err
	}
	decisions, err := rederiveAblationDecisions(root.Calls, store, role)
	if err != nil {
		return err
	}
	if err := verifyAblationActionEvidence(root, decisions); err != nil {
		return err
	}
	return store.requireClosure()
}

func verifyAblationTransitions(root ablationEvidenceRoot) error {
	if root.Transitions == nil || len(root.Transitions) == 0 ||
		len(root.Transitions) > maxEpisodeTraceEntries {
		return fmt.Errorf("ablation evidence transition history is invalid")
	}
	seenObservations := make(map[cognition.ObservationID]struct{})
	for index, transition := range root.Transitions {
		if index == 0 {
			if err := transition.ValidateStart(); err != nil ||
				transition.Current.EpisodeID != root.EpisodeID {
				return fmt.Errorf("ablation evidence start transition is invalid: %v", err)
			}
		} else if err := verifyAblationAppliedTransition(
			root.EpisodeID, root.Transitions[index-1], transition,
		); err != nil {
			return fmt.Errorf("ablation evidence transition %d: %w", index+1, err)
		}
		if index < len(root.Transitions)-1 && transition.Terminal {
			return fmt.Errorf("ablation evidence continued after a terminal transition")
		}
		for _, observation := range transition.Observations {
			if _, duplicate := seenObservations[observation.ID]; duplicate {
				return fmt.Errorf("ablation evidence reused an observation identity")
			}
			seenObservations[observation.ID] = struct{}{}
		}
	}
	last := root.Transitions[len(root.Transitions)-1]
	if last.Current != root.Terminal.Revision {
		return fmt.Errorf("ablation evidence terminal revision differs from transition history")
	}
	if root.Terminal.GoalSatisfied {
		if !last.Terminal || last.PublicOutcome != root.Terminal.PublicOutcome {
			return fmt.Errorf("successful ablation evidence lacks its terminal world transition")
		}
	} else if last.Terminal {
		return fmt.Errorf("failed ablation evidence claims a terminal world transition")
	}
	return nil
}

func verifyAblationAppliedTransition(
	episode cognition.EpisodeID,
	previous cognition.Transition,
	current cognition.Transition,
) error {
	if current.Previous == nil || *current.Previous != previous.Current ||
		current.ActionID == "" || current.Current.EpisodeID != episode ||
		current.Current.Number != previous.Current.Number+1 || current.Current.Validate() != nil ||
		current.Cost < 0 || current.Cost > cognition.MaxTransitionCost ||
		(current.Terminal && current.PublicOutcome == "") {
		return fmt.Errorf("world revision chain is invalid")
	}
	for index, observation := range current.Observations {
		if observation.Validate() != nil || observation.Revision != current.Current ||
			observation.ActionID != current.ActionID {
			return fmt.Errorf("observation %d is not bound to its transition", index+1)
		}
	}
	for index, effect := range current.Effects {
		if effect.Validate() != nil || effect.Revision != current.Current ||
			effect.ActionID != current.ActionID {
			return fmt.Errorf("effect %d is not bound to its transition", index+1)
		}
	}
	return nil
}

func verifyAblationLedgerEvidence(
	variant Variant,
	value *ablationLedgerEvidence,
) error {
	if value == nil {
		if ledgerBackedAblation(variant) {
			return fmt.Errorf("ledger-backed ablation evidence lacks its Task Ledger")
		}
		return nil
	}
	if !ledgerBackedAblation(variant) || value.Events == nil {
		return fmt.Errorf("ablation evidence contains an unexpected Task Ledger")
	}
	reconstructed, err := taskstate.Reconstruct(value.ID, value.Owner, value.Events)
	if err != nil || !reflect.DeepEqual(reconstructed.MaterializedState(), value.Terminal) {
		return fmt.Errorf("ablation evidence Task Ledger history changed: %v", err)
	}
	return nil
}

func verifyAblationWorkingSetEvidence(
	variant Variant,
	value *ablationWorkingSetEvidence,
) error {
	required := variant == VariantLedgerWorkingSet || variant == VariantLedgerProjection
	if value == nil {
		if required {
			return fmt.Errorf("Working-Set ablation evidence lacks its state")
		}
		return nil
	}
	if !required || value.Events == nil {
		return fmt.Errorf("ablation evidence contains an unexpected Working Set")
	}
	reconstructed, err := workingset.Restore(value.Initial)
	if err != nil {
		return fmt.Errorf("restore ablation evidence Working Set: %w", err)
	}
	for index, supplied := range value.Events {
		command, err := workingset.DecodeCommand(supplied.CommandKind, supplied.Command)
		if err != nil {
			return fmt.Errorf("decode ablation evidence Working Set event %d: %w", index+1, err)
		}
		actual, err := reconstructed.Apply(command)
		if err != nil || !reflect.DeepEqual(actual, supplied) {
			return fmt.Errorf("ablation evidence Working Set event %d changed: %v", index+1, err)
		}
	}
	if !reflect.DeepEqual(reconstructed.Snapshot(), value.Terminal) {
		return fmt.Errorf("ablation evidence Working Set terminal state changed")
	}
	return nil
}
