package cognitionstate

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

type ActionBinding struct {
	Action cognition.RegisteredAction
	Schema cognition.ActionSchema
}

func (binding ActionBinding) Validate() error {
	if err := binding.Action.Validate(binding.Schema); err != nil {
		return fmt.Errorf("%w: action binding: %v", ErrInvalidMapping, err)
	}
	return nil
}

type EnvironmentObservationInput struct {
	Ledger      taskstate.MaterializedState
	ScopeNodeID taskstate.NodeID
	Observation cognition.Observation
	Action      *ActionBinding
}

func MapEnvironmentObservation(input EnvironmentObservationInput) (EntryMutation, error) {
	if err := input.Observation.Validate(); err != nil {
		return EntryMutation{}, fmt.Errorf("%w: observation: %v", ErrInvalidMapping, err)
	}
	if input.Observation.ActionID == "" && input.Action != nil {
		return EntryMutation{}, fmt.Errorf("%w: initial observation cannot name an action binding", ErrInvalidMapping)
	}
	if input.Observation.ActionID != "" {
		if input.Action == nil {
			return EntryMutation{}, fmt.Errorf("%w: action observation requires its exact action binding", ErrInvalidMapping)
		}
		if err := input.Action.Validate(); err != nil {
			return EntryMutation{}, err
		}
		if input.Action.Action.ID != input.Observation.ActionID {
			return EntryMutation{}, fmt.Errorf("%w: observation and action identities differ", ErrInvalidMapping)
		}
	}
	refs := []taskstate.Ref{
		evidenceLedgerRef(input.Observation.EvidenceRef()), revisionLedgerRef(input.Observation.Revision),
	}
	metadataFields := map[string]any{
		"observation_id": input.Observation.ID, "revision": input.Observation.Revision,
	}
	if input.Action != nil {
		actionRef, err := actionLedgerRef(input.Observation.Revision.EpisodeID, *input.Action)
		if err != nil {
			return EntryMutation{}, err
		}
		refs = append(refs, actionRef)
		metadataFields["action_id"] = input.Action.Action.ID
	}
	mutation, err := newEntryMutation(entryCommandInput{
		Ledger: input.Ledger, ScopeNodeID: input.ScopeNodeID,
		SourceKind: SourceEnvironmentObservation,
		Source: struct {
			Observation cognition.Observation `json:"observation"`
			Action      *ActionBinding        `json:"action,omitempty"`
		}{input.Observation, input.Action},
		Actor: taskstate.AuthorityToolEvidence, Kind: taskstate.EntryObservation,
		Content: input.Observation.Content, Refs: refs, Metadata: metadataFields,
		ExpectedVersion: input.Ledger.Version,
	})
	if err != nil {
		return EntryMutation{}, err
	}
	if err := validateEntryMutation(input.Ledger, mutation); err != nil {
		return EntryMutation{}, err
	}
	return mutation, nil
}

type ActionFailureInput struct {
	Ledger           taskstate.MaterializedState
	ScopeNodeID      taskstate.NodeID
	Binding          ActionBinding
	ExpectedRevision cognition.WorldRevision
	Failure          cognition.ActionFailure
}

func MapActionFailure(input ActionFailureInput) (EntryMutation, error) {
	if err := input.Binding.Validate(); err != nil {
		return EntryMutation{}, err
	}
	if err := input.Failure.Validate(input.Binding.Action, input.ExpectedRevision); err != nil {
		return EntryMutation{}, fmt.Errorf("%w: action failure: %v", ErrInvalidMapping, err)
	}
	source := struct {
		Action   cognition.RegisteredAction `json:"action"`
		Expected cognition.WorldRevision    `json:"expected_revision"`
		Failure  cognition.ActionFailure    `json:"failure"`
	}{input.Binding.Action, input.ExpectedRevision, input.Failure.Clone()}
	sourceSHA, err := mappingDigest(source)
	if err != nil {
		return EntryMutation{}, err
	}
	actionRef, err := actionLedgerRef(input.ExpectedRevision.EpisodeID, input.Binding)
	if err != nil {
		return EntryMutation{}, err
	}
	refs := []taskstate.Ref{{
		URI:     "cognition:episode/" + string(input.ExpectedRevision.EpisodeID) + "/failure/" + string(input.Failure.ActionID),
		Version: fmt.Sprint(input.ExpectedRevision.Number), Hash: sourceSHA, Relation: taskstate.RefEvidence,
	}, revisionLedgerRef(input.ExpectedRevision), actionRef}
	for _, ref := range input.Failure.EvidenceRefs {
		refs = append(refs, evidenceLedgerRef(ref))
	}
	mutation, err := newEntryMutation(entryCommandInput{
		Ledger: input.Ledger, ScopeNodeID: input.ScopeNodeID,
		SourceKind: SourceActionFailure, Source: source,
		Actor: taskstate.AuthorityToolEvidence, Kind: taskstate.EntryFailure,
		Content: input.Failure.PublicMessage, Refs: refs,
		Metadata: map[string]any{
			"action_id": input.Failure.ActionID, "failure_code": input.Failure.Code,
			"revision": input.Failure.Revision,
		},
		ExpectedVersion: input.Ledger.Version,
	})
	if err != nil {
		return EntryMutation{}, err
	}
	if err := validateEntryMutation(input.Ledger, mutation); err != nil {
		return EntryMutation{}, err
	}
	return mutation, nil
}
