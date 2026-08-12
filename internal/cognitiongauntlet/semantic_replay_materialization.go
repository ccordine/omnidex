package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
)

func (state *semanticReplayState) planObligationMaterialization(
	reconciliationID string,
	snapshot cognition.RuntimeSnapshot,
	command cognitionruntime.ReconciliationCommand,
) error {
	proposalIndex := -1
	for index, proposal := range command.Decision.Proposals {
		if proposal.Kind != cognition.ProposalObligation {
			continue
		}
		if proposalIndex >= 0 {
			return fmt.Errorf("semantic decision proposes multiple obligation materializations")
		}
		proposalIndex = index
	}
	if proposalIndex < 0 {
		return nil
	}
	schema, exists := snapshot.ActionCatalog().Schema(command.Decision.Action.Kind)
	if !exists || schema.Ref() != command.ActionSchema.Ref() {
		return fmt.Errorf("semantic obligation materialization action schema changed")
	}
	matchVersion := state.activeGraphVersion + 1
	before, beforeExists := state.graphs[state.activeGraphVersion]
	after, afterExists := state.graphs[matchVersion]
	if !beforeExists || !afterExists {
		return nil
	}
	value, err := cognitionstate.MaterializeObligationProposal(
		cognitionstate.ObligationProposalInput{
			Graph: before, Snapshot: snapshot, Decision: command.Decision,
			ActionSchema: schema, ProposalIndex: proposalIndex,
			CompletionAuthority: state.completionAuthority,
		},
	)
	if err != nil || state.graphRecordIDs[matchVersion] != value.ID ||
		after.SHA256 != value.ResultGraphSHA256 {
		return nil
	}
	applied, err := value.Apply(before)
	if err != nil || !reflect.DeepEqual(applied, after) {
		return nil
	}
	if _, classified := state.classifiedGraphs[matchVersion]; classified {
		return fmt.Errorf("semantic obligation graph has duplicate mutation authority")
	}
	state.classifiedGraphs[matchVersion] = "obligation_materialization"
	if _, duplicate := state.graphMutations[reconciliationID]; duplicate {
		return fmt.Errorf("semantic reconciliation has two graph mutations")
	}
	state.graphMutations[reconciliationID] = semanticGraphMutation{
		version: matchVersion, kind: "obligation_materialization",
	}
	return nil
}
