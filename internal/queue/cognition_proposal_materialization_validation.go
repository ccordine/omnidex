package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/taskstate"
)

func cognitionProposalMaterializationTupleError(
	value CognitionProposalMaterialization,
	wantSource cognitionstate.SourceKind,
	wantEntry taskstate.EntryKind,
	descriptor taskstate.CommandDescriptor,
	commandErr error,
	registered bool,
) error {
	return fmt.Errorf(
		"%w: proposal materialization tuple is invalid: registered=%t source=%q/%q "+
			"pre_version=%d descriptor_version=%d command_version=%d descriptor_actor=%q "+
			"descriptor_kind=%q descriptor_sha=%q/%q command_kind=%q/%q entry=%q ledger=%q "+
			"id=%q self=%q snapshot=%q decision=%q policy=%q reconciliation=%q call=%d "+
			"output=%d status=%q episode=%q command_actor=%q descriptor_ids=%q/%q/%q/%q "+
			"replay_source=%q replay_sha=%q ledger_status=%q id_prefix=%t policy_prefix=%t reconciliation_prefix=%t "+
			"source_sha=%q mapping_id=%q mapping_sha=%q command_error=%v metadata_error=%v",
		ErrCognitionConflict, registered, value.SourceKind, wantSource,
		value.PreProposalLedgerVersion, value.ReplayDescriptor.ExpectedVersion,
		value.Command.ExpectedVersion, descriptor.Actor, descriptor.Kind, descriptor.SHA256,
		value.ReplayDescriptor.CommandSHA256, value.Command.Kind, wantEntry, value.EntryURI,
		value.ReplayDescriptor.LedgerID, value.ID, value.SHA256, value.SnapshotSHA256,
		value.DecisionSHA256, value.PolicyCallID, value.ReconciliationID, value.CallOrdinal,
		value.OutputLedgerVersion, value.OutputLedgerStatus, value.EpisodeID, value.Command.Actor,
		value.ReplayDescriptor.EntryID, value.Command.ID, value.ReplayDescriptor.CommandID,
		value.Command.CommandID, value.ReplayDescriptor.SourceKind, value.ReplayDescriptor.LedgerSHA256,
		value.PreProposalLedger.Status, len(value.ID) > len(cognitionProposalMaterializationPrefix),
		len(value.PolicyCallID) > len("cognition_call_"),
		len(value.ReconciliationID) > len("cognition_reconciliation_"), value.ReplayDescriptor.SourceSHA256,
		value.ReplayDescriptor.ID, value.ReplayDescriptor.SHA256, commandErr, value.Command.Metadata.Validate(),
	)
}
