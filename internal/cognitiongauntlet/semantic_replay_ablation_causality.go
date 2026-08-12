package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func verifyAblationSemanticStateCausality(root ablationEvidenceRoot) error {
	observations := ablationOrderedObservations(root.Transitions)
	if root.Ledger == nil {
		if root.WorkingSet != nil {
			return fmt.Errorf("ablation Working Set lacks its causal Task Ledger")
		}
		return nil
	}
	wantOwner := taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: root.Actor.JobID,
		RunID: ablationLedgerRunID(root.EpisodeID),
	}
	wantLedgerID, err := taskstate.NewLedgerID(wantOwner)
	if err != nil || root.Ledger.Owner != wantOwner || root.Ledger.ID != wantLedgerID {
		return fmt.Errorf("ablation ledger genesis differs from code authority: %v", err)
	}
	ledger, err := taskstate.NewLedger(wantLedgerID, wantOwner)
	if err != nil || len(root.Ledger.Events) != len(observations) {
		return fmt.Errorf("ablation ledger does not map every observation exactly once: %v", err)
	}
	for index, observation := range observations {
		entryID := taskstate.EntryID("observation-entry-" + string(observation.ID))
		commandID, err := taskstate.NewCommandID(
			string(root.EpisodeID), string(observation.ID), fmt.Sprint(ledger.Version()+1),
		)
		if err != nil {
			return err
		}
		actual, err := ledger.Apply(taskstate.AddEntryCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(),
			Actor: taskstate.AuthorityToolEvidence, ID: entryID,
			Kind: taskstate.EntryObservation, Content: observation.Content,
			Metadata: taskstate.EmptyJSONObject(),
			Refs:     []taskstate.Ref{ablationObservationRef(observation)},
		})
		gotSHA, gotErr := digestJSON(actual)
		wantSHA, wantErr := digestJSON(root.Ledger.Events[index])
		if err != nil || gotErr != nil || wantErr != nil || gotSHA != wantSHA {
			return fmt.Errorf(
				"ablation ledger event %d is not the exact observation projection: %v / %v / %v",
				index+1, err, gotErr, wantErr,
			)
		}
	}
	if !reflect.DeepEqual(ledger.MaterializedState(), root.Ledger.Terminal) {
		return fmt.Errorf("ablation causal ledger terminal changed")
	}
	return verifyAblationSemanticWorkingSetCausality(root, observations)
}

func verifyAblationSemanticWorkingSetCausality(
	root ablationEvidenceRoot,
	observations []cognition.Observation,
) error {
	if root.WorkingSet == nil {
		return nil
	}
	if len(root.WorkingSet.Events) != len(observations) || root.Ledger == nil {
		return fmt.Errorf("ablation Working Set does not map every ledger observation exactly once")
	}
	wantSet, err := workingset.New(workingset.Owner{
		LedgerID: root.Ledger.ID, JobID: root.Actor.JobID, Generation: root.Actor.Generation,
	}, workingset.Budget{
		MaxItems: 64, MaxBytes: root.PublicRunAuthority.Budget.WorkingSetBytes,
		MaxPinnedItems: 0, MaxPinnedBytes: 0,
	})
	if err != nil || !reflect.DeepEqual(root.WorkingSet.Initial, wantSet.Snapshot()) {
		return fmt.Errorf("ablation Working Set genesis differs from code authority: %v", err)
	}
	set, err := workingset.Restore(root.WorkingSet.Initial)
	if err != nil {
		return err
	}
	for index, observation := range observations {
		ledgerEvent := root.Ledger.Events[index]
		entryID := taskstate.EntryID("observation-entry-" + string(observation.ID))
		ref := ablationContentRef(
			"cognition:episode/"+string(root.EpisodeID)+"/ledger-entry/"+string(entryID),
			fmt.Sprint(ledgerEvent.Version), observation.Content, taskstate.RefEvidence,
		)
		commandID, err := workingset.NewCommandID(
			string(root.EpisodeID), string(observation.ID), "retain-observation",
		)
		if err != nil {
			return err
		}
		actual, err := set.Apply(workingset.AcquireCommand{
			CommandID: commandID, ExpectedVersion: set.Version(), Actor: taskstate.AuthorityCode,
			Request: workingset.AcquireRequest{
				ID:  workingset.ItemID("resident-material-" + fmt.Sprint(index+1)),
				Ref: ref, Role: workingset.RoleEvidence, Retention: workingset.RetentionJob,
				Scope: set.Scope(), Priority: 50, ByteCost: len([]byte(observation.Content)),
				Acquisition: workingset.Acquisition{
					Provider:    workingset.ProviderTaskState,
					OperationID: string(ledgerEvent.CommandID),
					Reason:      "Retain exact observed evidence while it supports the active objective.",
				},
			},
		})
		if err != nil || !reflect.DeepEqual(actual, root.WorkingSet.Events[index]) {
			return fmt.Errorf("ablation Working Set event %d is not the causal ledger projection: %v", index+1, err)
		}
	}
	if !reflect.DeepEqual(set.Snapshot(), root.WorkingSet.Terminal) {
		return fmt.Errorf("ablation causal Working Set terminal changed")
	}
	return nil
}

func ablationOrderedObservations(
	transitions []cognition.Transition,
) []cognition.Observation {
	values := make([]cognition.Observation, 0)
	for _, transition := range transitions {
		for _, observation := range transition.Observations {
			values = append(values, observation)
		}
	}
	return values
}
