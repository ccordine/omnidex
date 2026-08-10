package cognitionstate

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func MapSelectedDecisionCandidate(input ModelProposalInput) (EntryMutation, error) {
	if _, err := MapModelProposals(input); err != nil {
		return EntryMutation{}, err
	}
	contentRaw, err := json.Marshal(struct {
		Action         cognition.ActionRequest `json:"action"`
		ExpectedEffect string                  `json:"expected_effect"`
	}{input.Decision.Action.Clone(), input.Decision.ExpectedEffect})
	if err != nil {
		return EntryMutation{}, fmt.Errorf("%w: selected decision content: %v", ErrInvalidMapping, err)
	}
	refs := make([]taskstate.Ref, len(input.Decision.EvidenceRefs))
	for index, ref := range input.Decision.EvidenceRefs {
		refs[index] = evidenceLedgerRef(ref)
	}
	return newEntryMutation(entryCommandInput{
		Ledger: input.Ledger, ScopeNodeID: input.ScopeNodeID, SourceKind: SourceModelDecision,
		Source: struct {
			SnapshotSHA256 string                         `json:"snapshot_sha256"`
			Attempt        cognition.AttemptRef           `json:"attempt"`
			Projection     cognition.ContextProjectionRef `json:"context_projection"`
			ActionSchema   cognition.ActionSchemaRef      `json:"action_schema"`
			Decision       cognition.CognitionDecision    `json:"decision"`
		}{
			input.Snapshot.SHA256(), input.Snapshot.Attempt(), input.Snapshot.ContextProjection(),
			input.ActionSchema.Ref(), input.Decision.Clone(),
		},
		Actor: taskstate.AuthorityModelProposal, Kind: taskstate.EntryDecisionCandidate,
		Content: string(contentRaw), Refs: refs,
		Metadata: map[string]any{
			"candidate_kind": "selected_action", "attempt": input.Snapshot.Attempt(),
			"context_projection": input.Snapshot.ContextProjection(),
			"snapshot_sha256":    input.Snapshot.SHA256(), "action_schema": input.ActionSchema.Ref(),
		},
		ExpectedVersion: input.Ledger.Version,
	})
}
