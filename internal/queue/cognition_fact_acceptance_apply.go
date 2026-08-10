package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func persistCognitionTransitionFactsTx(
	ctx context.Context,
	tx pgx.Tx,
	header taskLedgerHeader,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	obligationID cognition.ObligationID,
	transition cognition.Transition,
	facts cognitionstate.FactAcceptanceAuthority,
) (taskLedgerHeader, error) {
	restored, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return header, err
	}
	mutations, err := facts.MapTransitionFacts(
		restored.MaterializedState(), taskstate.NodeID(obligationID), transition,
	)
	if err != nil {
		return header, fmt.Errorf("plan cognition accepted facts: %w", err)
	}
	_, transitionSHA, err := cognitionJSON(transition)
	if err != nil {
		return header, err
	}
	transitionID := cognitionTransitionID(episodeID, transitionSHA)
	for _, mutation := range mutations {
		command := mutation.Command()
		policy, evidence, err := acceptedFactCommandAuthority(command, transition)
		if err != nil {
			return header, err
		}
		record := cognitionAcceptedFact{
			Schema: cognitionAcceptedFactSchemaV1, EpisodeID: episodeID, LedgerID: header.ID,
			TransitionID: transitionID, TransitionSHA256: transitionSHA,
			ScopeObligationID: obligationID, AuthoritySHA256: facts.Reference().SHA256,
			Planner: facts.Reference().Planner, Policy: policy,
			EvidenceRefs: evidence, Mapping: mutation.Descriptor(),
		}
		_, record.SHA256, err = cognitionJSON(record.identity())
		if err != nil {
			return header, err
		}
		record.ID = "cognition_accepted_fact_" + record.SHA256
		if err := record.validate(); err != nil {
			return header, err
		}
		event, err := applyQueueOwnedTaskCommandTx(
			ctx, tx, authority.JobID, authority.Generation, command,
		)
		if err != nil {
			return header, fmt.Errorf("persist cognition accepted fact: %w", err)
		}
		header.Version = event.Version
		if err := insertCognitionAcceptedFactTx(ctx, tx, record); err != nil {
			return header, err
		}
	}
	return header, nil
}

func acceptedFactCommandAuthority(
	command taskstate.AddEntryCommand,
	transition cognition.Transition,
) (cognitionstate.FactAcceptancePolicyRef, []cognition.EvidenceRef, error) {
	var metadata struct {
		Policy cognitionstate.FactAcceptancePolicyRef `json:"acceptance_policy"`
	}
	if err := json.Unmarshal(command.Metadata.Bytes(), &metadata); err != nil || metadata.Policy.Validate() != nil {
		return cognitionstate.FactAcceptancePolicyRef{}, nil,
			fmt.Errorf("%w: accepted fact policy metadata is invalid", ErrCognitionConflict)
	}
	available := make(map[taskstate.Ref]cognition.EvidenceRef, len(transition.Observations))
	for _, observation := range transition.Observations {
		ref := observation.EvidenceRef()
		available[cognitionEvidenceTaskRefs([]cognition.EvidenceRef{ref})[0]] = ref
	}
	evidence := make([]cognition.EvidenceRef, len(command.Refs))
	for index, ref := range command.Refs {
		value, exists := available[ref]
		if !exists {
			return cognitionstate.FactAcceptancePolicyRef{}, nil,
				fmt.Errorf("%w: accepted fact cites evidence outside its transition", ErrCognitionConflict)
		}
		evidence[index] = value
	}
	return metadata.Policy, evidence, nil
}
