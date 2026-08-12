package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
)

func newCognitionProposalMaterializations(
	episodeID cognition.EpisodeID,
	policyCallID string,
	callOrdinal uint64,
	input cognitionstate.ModelProposalInput,
	receipt cognitionruntime.ReconciliationReceipt,
) ([]CognitionProposalMaterialization, error) {
	decisionSHA, err := cognitionruntime.DecisionSHA256(input.Decision)
	if err != nil {
		return nil, err
	}
	if receipt.ID == "" || receipt.SnapshotSHA256 != input.Snapshot.SHA256() ||
		receipt.DecisionSHA256 != decisionSHA || receipt.ActionSchema != input.ActionSchema.Ref() ||
		episodeID != input.Snapshot.CurrentRevision().EpisodeID {
		return nil, fmt.Errorf("%w: proposal materialization reconciliation authority changed", ErrCognitionConflict)
	}
	ledgerSHA, err := cognitionProposalLedgerSHA(input.Ledger)
	if err != nil {
		return nil, err
	}
	ledgerJSONSHA, err := cognitionProposalLedgerJSONSHA(input.Ledger)
	if err != nil {
		return nil, err
	}
	mutations, err := cognitionstate.MapModelProposals(input)
	if err != nil {
		return nil, err
	}
	decision := input.Decision.Clone()
	values := make([]CognitionProposalMaterialization, 0, len(mutations))
	seen := make(map[int]struct{}, len(mutations))
	for _, mutation := range mutations {
		descriptor, command := mutation.Descriptor(), mutation.Command()
		if descriptor.ExpectedVersion < input.Ledger.Version {
			return nil, fmt.Errorf("%w: proposal mapping precedes its input ledger", ErrCognitionConflict)
		}
		index := int(descriptor.ExpectedVersion - input.Ledger.Version)
		if index < 0 || index >= len(decision.Proposals) {
			return nil, fmt.Errorf("%w: proposal mapping index is unavailable", ErrCognitionConflict)
		}
		if _, duplicate := seen[index]; duplicate {
			return nil, fmt.Errorf("%w: proposal mapping index %d is duplicated", ErrCognitionConflict, index)
		}
		seen[index] = struct{}{}
		value := CognitionProposalMaterialization{
			Schema: CognitionProposalMaterializationSchemaV1, EpisodeID: episodeID,
			ReconciliationID: receipt.ID, PolicyCallID: policyCallID, CallOrdinal: callOrdinal,
			SnapshotSHA256: input.Snapshot.SHA256(), DecisionSHA256: decisionSHA,
			ProposalIndex: index, Proposal: decision.Proposals[index], SourceKind: descriptor.SourceKind,
			PreProposalLedgerVersion: input.Ledger.Version, PreProposalLedgerSHA256: ledgerSHA,
			PreProposalLedgerJSONSHA256: ledgerJSONSHA,
			PreProposalLedger:           input.Ledger,
			ReplayDescriptor:            descriptor, Command: command,
			EntryURI:            cognitionProposalEntryURI(descriptor.LedgerID, command.ID),
			OutputLedgerVersion: command.ExpectedVersion + 1, OutputLedgerStatus: taskstate.LedgerActive,
		}
		value.SHA256, err = cognitionProposalCanonicalSHA(value.identity())
		if err != nil {
			return nil, err
		}
		value.ID = cognitionProposalMaterializationPrefix + value.SHA256
		if err := VerifyCognitionProposalMaterialization(value, input); err != nil {
			return nil, fmt.Errorf("proposal materialization %d (%s): %w", index, value.Proposal.Kind, err)
		}
		values = append(values, value)
	}
	expected := 0
	for _, proposal := range decision.Proposals {
		if proposal.Kind != cognition.ProposalRevision {
			expected++
		}
	}
	if len(values) != expected {
		return nil, fmt.Errorf("%w: proposal materialization count changed", ErrCognitionConflict)
	}
	if len(values) > 0 && values[len(values)-1].OutputLedgerVersion != receipt.LedgerVersion {
		return nil, fmt.Errorf("%w: proposal materializations do not reach the reconciliation ledger version", ErrCognitionConflict)
	}
	return values, nil
}
