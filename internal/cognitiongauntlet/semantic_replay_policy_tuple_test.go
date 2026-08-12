package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticPolicyEvidenceRequiresItsExactCallTuple(t *testing.T) {
	response, err := cognitionpolicy.NewModelResponseEvidence("call-one", "model response")
	if err != nil {
		t.Fatal(err)
	}
	metadata, present, err := semanticPolicyMetadataForRef(
		"model_response", response.CallID, response.Ref,
	)
	if err != nil || !present {
		t.Fatal(err)
	}
	record := semanticReplayRawRecord(
		"policy_response_evidence", 1, 32, 0, metadata.EvidenceID,
		semanticReplayJSON(t, metadata),
	)
	valid := func(value queue.CognitionSealedTraceRecord) error {
		state := &semanticReplayState{attemptOrdinals: map[int64]string{1: "call-one"}}
		_, err := state.mapOpaquePolicyEvidence(
			value, semanticReplaySourceForRecord(t, 1, value),
		)
		return err
	}
	if err := valid(record); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*queue.CognitionSealedTraceRecord){
		"call ordinal": func(value *queue.CognitionSealedTraceRecord) { value.CallOrdinal++ },
		"phase":        func(value *queue.CognitionSealedTraceRecord) { value.Phase++ },
		"sequence":     func(value *queue.CognitionSealedTraceRecord) { value.Sequence++ },
	} {
		t.Run(name, func(t *testing.T) {
			changed := record
			mutate(&changed)
			if err := valid(changed); err == nil {
				t.Fatal("semantic policy evidence accepted a changed call tuple")
			}
		})
	}
	state := &semanticReplayState{attemptOrdinals: map[int64]string{1: "call-two"}}
	if _, err := state.mapOpaquePolicyEvidence(
		record, semanticReplaySourceForRecord(t, 1, record),
	); err == nil {
		t.Fatal("semantic policy evidence was swapped between calls")
	}
}

func TestSemanticRevisionProposalRequiresCodeMaterialization(t *testing.T) {
	_, _, err := semanticMaterializationKnowledge(queue.CognitionProposalMaterialization{
		Proposal: cognition.LedgerProposal{Kind: cognition.ProposalRevision},
	})
	if err == nil {
		t.Fatal("revision proposal minted knowledge without code materialization")
	}
}

func TestSemanticBeliefRevisionFailsWithoutExactProposalMaterialization(t *testing.T) {
	state := &semanticReplayState{}
	_, err := state.mapBeliefRevision(
		queue.CognitionSealedTraceRecord{Kind: "belief_revision"},
		cognitionreplay.SourceRecord{},
	)
	if err == nil || !strings.Contains(err.Error(), "proposal-materialization evidence") {
		t.Fatalf("belief revision without stable materialization authority error=%v", err)
	}
}
