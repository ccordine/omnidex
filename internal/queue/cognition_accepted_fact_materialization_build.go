package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/taskstate"
)

func newCognitionAcceptedFactMaterialization(
	episodeID cognition.EpisodeID,
	scope cognition.ObligationID,
	transition cognition.Transition,
	authority cognitionstate.FactAcceptanceAuthority,
	preLedger taskstate.MaterializedState,
	callOrdinal uint64,
) (CognitionAcceptedFactMaterialization, error) {
	if err := authority.Validate(); err != nil {
		return CognitionAcceptedFactMaterialization{}, err
	}
	if transition.Current.EpisodeID != episodeID ||
		(transition.ActionID == "") != (callOrdinal == 0) {
		return CognitionAcceptedFactMaterialization{}, fmt.Errorf(
			"%w: accepted-fact materialization transition tuple changed",
			ErrCognitionConflict,
		)
	}
	transition = transition.Clone()
	_, transitionSHA, err := cognitionJSON(transition)
	if err != nil {
		return CognitionAcceptedFactMaterialization{}, err
	}
	_, ledgerSHA, err := cognitionJSON(preLedger)
	if err != nil {
		return CognitionAcceptedFactMaterialization{}, err
	}
	ledgerRaw, err := exactjson.Canonical(preLedger)
	if err != nil {
		return CognitionAcceptedFactMaterialization{}, err
	}
	mutations, err := authority.MapTransitionFacts(
		preLedger, taskstate.NodeID(scope), transition,
	)
	if err != nil {
		return CognitionAcceptedFactMaterialization{}, err
	}
	if len(mutations) > MaxCognitionAcceptedFactsPerTransition {
		return CognitionAcceptedFactMaterialization{}, fmt.Errorf(
			"%w: transition produced %d accepted facts; hard limit is %d",
			ErrCognitionConflict, len(mutations), MaxCognitionAcceptedFactsPerTransition,
		)
	}
	transitionID := cognitionTransitionID(episodeID, transitionSHA)
	members := make([]CognitionAcceptedFactMaterializationMember, 0, len(mutations))
	for index, mutation := range mutations {
		command := mutation.Command()
		policy, evidence, err := acceptedFactCommandAuthority(command, transition)
		if err != nil {
			return CognitionAcceptedFactMaterialization{}, err
		}
		fact := CognitionAcceptedFactTrace{
			Schema: cognitionAcceptedFactSchemaV1, EpisodeID: episodeID, LedgerID: preLedger.ID,
			TransitionID: transitionID, TransitionSHA256: transitionSHA,
			ScopeObligationID: scope, AuthoritySHA256: authority.Reference().SHA256,
			Planner: authority.Reference().Planner, Policy: policy,
			EvidenceRefs: evidence, Mapping: mutation.Descriptor(),
		}
		_, fact.SHA256, err = cognitionJSON(fact.identity())
		if err != nil {
			return CognitionAcceptedFactMaterialization{}, err
		}
		fact.ID = "cognition_accepted_fact_" + fact.SHA256
		members = append(members, CognitionAcceptedFactMaterializationMember{
			Index: index, Fact: fact, Command: command,
			EntryURI:            "task:ledger/" + string(preLedger.ID) + "/entry/" + string(command.ID),
			OutputLedgerVersion: command.ExpectedVersion + 1,
			OutputLedgerStatus:  taskstate.LedgerActive,
		})
	}
	value := CognitionAcceptedFactMaterialization{
		Schema:    CognitionAcceptedFactMaterializationSchemaV1,
		EpisodeID: episodeID, LedgerID: preLedger.ID,
		TransitionID: transitionID, TransitionSHA256: transitionSHA,
		TransitionRevision: transition.Current.Number,
		ActionID:           transition.ActionID, CallOrdinal: callOrdinal, ScopeObligationID: scope,
		FactAuthority: authority.Reference(), PreFactLedgerVersion: preLedger.Version,
		PreFactLedgerSHA256: ledgerSHA, PreFactLedgerJSONSHA256: cognitionPayloadSHA(ledgerRaw),
		PreFactLedger: preLedger, Members: members,
		OutputLedgerVersion: preLedger.Version + uint64(len(members)),
		OutputLedgerStatus:  taskstate.LedgerActive,
	}
	identityRaw, err := exactjson.Canonical(value.identity())
	if err != nil {
		return CognitionAcceptedFactMaterialization{}, err
	}
	value.SHA256 = cognitionPayloadSHA(identityRaw)
	value.ID = cognitionAcceptedFactMaterializationPrefix + value.SHA256
	if err := value.Validate(); err != nil {
		return CognitionAcceptedFactMaterialization{}, err
	}
	return value, nil
}
