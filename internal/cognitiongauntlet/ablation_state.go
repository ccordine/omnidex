package cognitiongauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

type ablationActionHistory struct {
	Request       cognition.ActionRequest `json:"request"`
	PublicOutcome string                  `json:"public_outcome"`
	Failed        bool                    `json:"failed"`
}

type ablationState struct {
	variant          Variant
	episode          cognition.EpisodeRef
	actor            cognition.AttemptRef
	goal             cognition.GoalExpression
	obligation       cognition.Obligation
	catalog          cognition.ActionCatalog
	observations     []cognition.Observation
	actions          []ablationActionHistory
	ledger           *taskstate.Ledger
	workingSet       *workingset.Set
	workingMaterials map[workingset.ItemID]ablationMaterial
	observationItems map[cognition.ObservationID]workingset.ItemID
}

func newAblationState(
	variant Variant,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	scenario labyrinth.Scenario,
	workingSetBytes int,
) (*ablationState, error) {
	goal := scenario.Goal()
	completion, err := labyrinth.NewCompletionAuthority(scenario)
	if err != nil {
		return nil, err
	}
	return newAblationStateWithAuthority(
		variant, episode, actor, goal, completion, scenario.Catalog(), workingSetBytes,
	)
}

func newAblationStateWithAuthority(
	variant Variant,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	goal cognition.GoalExpression,
	completion cognition.CompletionAuthority,
	catalog cognition.ActionCatalog,
	workingSetBytes int,
) (*ablationState, error) {
	if err := goal.Validate(); err != nil {
		return nil, err
	}
	if err := completion.Validate(); err != nil {
		return nil, err
	}
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	check, err := completion.Resolve(goal)
	if err != nil {
		return nil, err
	}
	generation := cognition.InitialObligationGeneration
	id, err := cognition.DeriveObligationID(episode.ID, generation, "", goal, check)
	if err != nil {
		return nil, err
	}
	graph, err := cognition.NewObligationGraph(generation, id, []cognition.ObligationSpec{{
		ID: id, Desired: goal, DependsOn: []cognition.ObligationID{},
		SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
	}})
	if err != nil {
		return nil, err
	}
	if err := graph.RefreshReadiness(generation); err != nil {
		return nil, err
	}
	if err := graph.Transition(id, generation, cognition.ObligationActive); err != nil {
		return nil, err
	}
	obligation, found := graph.Obligation(id)
	if !found {
		return nil, fmt.Errorf("ablation root obligation was not materialized")
	}
	state := &ablationState{
		variant: variant, episode: episode, actor: actor, goal: goal,
		obligation: obligation, catalog: catalog.Clone(),
		observations:     make([]cognition.Observation, 0, 32),
		actions:          make([]ablationActionHistory, 0, 32),
		workingMaterials: make(map[workingset.ItemID]ablationMaterial),
		observationItems: make(map[cognition.ObservationID]workingset.ItemID),
	}
	if !ledgerBackedAblation(variant) {
		return state, nil
	}
	owner := taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: actor.JobID,
		RunID: ablationLedgerRunID(episode.ID),
	}
	ledgerID, err := taskstate.NewLedgerID(owner)
	if err != nil {
		return nil, err
	}
	state.ledger, err = taskstate.NewLedger(ledgerID, owner)
	if err != nil {
		return nil, err
	}
	if variant == VariantLedgerWorkingSet || variant == VariantLedgerProjection {
		state.workingSet, err = workingset.New(workingset.Owner{
			LedgerID: ledgerID, JobID: actor.JobID, Generation: actor.Generation,
		}, workingset.Budget{
			MaxItems: 64, MaxBytes: workingSetBytes, MaxPinnedItems: 0, MaxPinnedBytes: 0,
		})
	}
	return state, err
}

func ablationLedgerRunID(episodeID cognition.EpisodeID) string {
	digest := sha256.Sum256([]byte(episodeID))
	digest[6] = (digest[6] & 0x0f) | 0x40
	digest[8] = (digest[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(digest[:16])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}

func ledgerBackedAblation(variant Variant) bool {
	return variant == VariantTaskLedger || variant == VariantLedgerWorkingSet ||
		variant == VariantLedgerProjection
}

func (state *ablationState) recordTransition(transition cognition.Transition) error {
	for _, observation := range transition.Observations {
		state.observations = append(state.observations, observation)
		if state.ledger != nil {
			if err := state.recordLedgerObservation(observation); err != nil {
				return err
			}
		}
	}
	return nil
}

func (state *ablationState) recordLedgerObservation(observation cognition.Observation) error {
	entryID := taskstate.EntryID("observation-entry-" + string(observation.ID))
	commandID, err := taskstate.NewCommandID(
		string(state.episode.ID), string(observation.ID), fmt.Sprint(state.ledger.Version()+1),
	)
	if err != nil {
		return err
	}
	rawRef := ablationObservationRef(observation)
	if _, err := state.ledger.Apply(taskstate.AddEntryCommand{
		CommandID: commandID, ExpectedVersion: state.ledger.Version(),
		Actor: taskstate.AuthorityToolEvidence, ID: entryID, Kind: taskstate.EntryObservation,
		Content: observation.Content, Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{rawRef},
	}); err != nil {
		return fmt.Errorf("record ablation Task Ledger observation: %w", err)
	}
	if state.workingSet == nil {
		return nil
	}
	content := observation.Content
	ref := ablationContentRef(
		"cognition:episode/"+string(state.episode.ID)+"/ledger-entry/"+string(entryID),
		fmt.Sprint(state.ledger.Version()), content, taskstate.RefEvidence,
	)
	itemID := workingset.ItemID("resident-material-" + fmt.Sprint(len(state.observationItems)+1))
	result, err := state.workingSet.Acquire(workingset.AcquireRequest{
		ID: itemID, Ref: ref, Role: workingset.RoleEvidence,
		Retention: workingset.RetentionJob, Scope: state.workingSet.Scope(),
		Priority: 50, ByteCost: len([]byte(content)),
		Acquisition: workingset.Acquisition{
			Provider: workingset.ProviderTaskState, OperationID: string(commandID),
			Reason: "Retain exact observed evidence while it supports the active objective.",
		},
	})
	if err != nil {
		return fmt.Errorf("retain ablation Working Set observation: %w", err)
	}
	for _, evicted := range result.Evicted {
		delete(state.workingMaterials, evicted.ID)
	}
	state.workingMaterials[itemID] = ablationMaterial{
		Ref: ref, SourceRefs: []taskstate.Ref{rawRef}, Role: workingset.RoleEvidence,
		Authority: taskstate.AuthorityToolEvidence, Content: content, Priority: 50,
	}
	state.observationItems[observation.ID] = itemID
	return nil
}

func (state *ablationState) taskMaterial() (ablationMaterial, error) {
	raw, err := json.Marshal(state.obligation)
	if err != nil {
		return ablationMaterial{}, err
	}
	content := string(raw)
	return ablationMaterial{
		Ref: ablationContentRef(
			"cognition:episode/"+string(state.episode.ID)+"/obligation/"+string(state.obligation.ID),
			fmt.Sprint(state.obligation.CreatedGeneration), content, taskstate.RefConcerns,
		),
		SourceRefs: []taskstate.Ref{}, Role: workingset.RoleTask,
		Authority: taskstate.AuthorityCode, Content: content, Priority: 100,
	}, nil
}

func (state *ablationState) appendAction(request cognition.ActionRequest, outcome string, failed bool) {
	state.actions = append(state.actions, ablationActionHistory{
		Request: request.Clone(), PublicOutcome: outcome, Failed: failed,
	})
}
