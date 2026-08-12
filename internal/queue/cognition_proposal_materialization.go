package queue

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	CognitionProposalMaterializationSchemaV1  = "omnidex.cognition-proposal-materialization.v1"
	CognitionTraceKindProposalMaterialization = "proposal_materialization"
	cognitionProposalMaterializationPrefix    = "cognition_proposal_materialization_"
)

type CognitionProposalMaterialization struct {
	Schema                      string                          `json:"schema"`
	ID                          string                          `json:"id"`
	SHA256                      string                          `json:"sha256"`
	EpisodeID                   cognition.EpisodeID             `json:"episode_id"`
	ReconciliationID            string                          `json:"reconciliation_id"`
	PolicyCallID                string                          `json:"policy_call_id"`
	CallOrdinal                 uint64                          `json:"call_ordinal"`
	SnapshotSHA256              string                          `json:"snapshot_sha256"`
	DecisionSHA256              string                          `json:"decision_sha256"`
	ProposalIndex               int                             `json:"proposal_index"`
	Proposal                    cognition.LedgerProposal        `json:"proposal"`
	SourceKind                  cognitionstate.SourceKind       `json:"source_kind"`
	PreProposalLedgerVersion    uint64                          `json:"pre_proposal_ledger_version"`
	PreProposalLedgerSHA256     string                          `json:"pre_proposal_ledger_sha256"`
	PreProposalLedgerJSONSHA256 string                          `json:"pre_proposal_ledger_json_sha256"`
	PreProposalLedger           taskstate.MaterializedState     `json:"pre_proposal_ledger"`
	ReplayDescriptor            cognitionstate.ReplayDescriptor `json:"replay_descriptor"`
	Command                     taskstate.AddEntryCommand       `json:"command"`
	EntryURI                    string                          `json:"entry_uri"`
	OutputLedgerVersion         uint64                          `json:"output_ledger_version"`
	OutputLedgerStatus          taskstate.LedgerStatus          `json:"output_ledger_status"`
}

type cognitionProposalMaterializationIdentity struct {
	Schema                      string                          `json:"schema"`
	EpisodeID                   cognition.EpisodeID             `json:"episode_id"`
	ReconciliationID            string                          `json:"reconciliation_id"`
	PolicyCallID                string                          `json:"policy_call_id"`
	CallOrdinal                 uint64                          `json:"call_ordinal"`
	SnapshotSHA256              string                          `json:"snapshot_sha256"`
	DecisionSHA256              string                          `json:"decision_sha256"`
	ProposalIndex               int                             `json:"proposal_index"`
	Proposal                    cognition.LedgerProposal        `json:"proposal"`
	SourceKind                  cognitionstate.SourceKind       `json:"source_kind"`
	PreProposalLedgerVersion    uint64                          `json:"pre_proposal_ledger_version"`
	PreProposalLedgerSHA256     string                          `json:"pre_proposal_ledger_sha256"`
	PreProposalLedgerJSONSHA256 string                          `json:"pre_proposal_ledger_json_sha256"`
	PreProposalLedger           taskstate.MaterializedState     `json:"pre_proposal_ledger"`
	ReplayDescriptor            cognitionstate.ReplayDescriptor `json:"replay_descriptor"`
	Command                     taskstate.AddEntryCommand       `json:"command"`
	EntryURI                    string                          `json:"entry_uri"`
	OutputLedgerVersion         uint64                          `json:"output_ledger_version"`
	OutputLedgerStatus          taskstate.LedgerStatus          `json:"output_ledger_status"`
}

func (value CognitionProposalMaterialization) identity() any {
	return cognitionProposalMaterializationIdentity{
		Schema: value.Schema, EpisodeID: value.EpisodeID, ReconciliationID: value.ReconciliationID,
		PolicyCallID: value.PolicyCallID, CallOrdinal: value.CallOrdinal,
		SnapshotSHA256: value.SnapshotSHA256, DecisionSHA256: value.DecisionSHA256,
		ProposalIndex: value.ProposalIndex, Proposal: value.Proposal, SourceKind: value.SourceKind,
		PreProposalLedgerVersion:    value.PreProposalLedgerVersion,
		PreProposalLedgerSHA256:     value.PreProposalLedgerSHA256,
		PreProposalLedgerJSONSHA256: value.PreProposalLedgerJSONSHA256,
		PreProposalLedger:           value.PreProposalLedger,
		ReplayDescriptor:            value.ReplayDescriptor, Command: value.Command, EntryURI: value.EntryURI,
		OutputLedgerVersion: value.OutputLedgerVersion, OutputLedgerStatus: value.OutputLedgerStatus,
	}
}

func (value CognitionProposalMaterialization) Validate() error {
	wantSource, wantEntry, ok := cognitionProposalMaterializationKinds(value.Proposal.Kind)
	descriptor, err := taskstate.DescribeCommand(value.Command)
	identitySHA, identityErr := cognitionProposalCanonicalSHA(value.identity())
	ledgerSHA, ledgerErr := cognitionProposalLedgerSHA(value.PreProposalLedger)
	ledgerJSONSHA, ledgerJSONErr := cognitionProposalLedgerJSONSHA(value.PreProposalLedger)
	ledgerValidationErr := taskstate.ValidateMaterializedState(value.PreProposalLedger)
	if identityErr != nil || value.SHA256 != identitySHA {
		return fmt.Errorf("%w: cognition proposal materialization identity is invalid: %v", ErrCognitionConflict, identityErr)
	}
	if ledgerErr != nil || ledgerJSONErr != nil || ledgerValidationErr != nil ||
		value.PreProposalLedger.Version != value.PreProposalLedgerVersion ||
		ledgerSHA != value.PreProposalLedgerSHA256 || ledgerJSONSHA != value.PreProposalLedgerJSONSHA256 {
		return fmt.Errorf(
			"%w: cognition proposal materialization ledger is invalid (validation=%v sha=%q/%q version=%d/%d)",
			ErrCognitionConflict, ledgerValidationErr, ledgerSHA, value.PreProposalLedgerSHA256,
			value.PreProposalLedger.Version, value.PreProposalLedgerVersion,
		)
	}
	if value.Schema != CognitionProposalMaterializationSchemaV1 ||
		!strings.HasPrefix(value.ID, cognitionProposalMaterializationPrefix) ||
		value.ID != cognitionProposalMaterializationPrefix+value.SHA256 ||
		!cognitionDigestPattern.MatchString(value.SHA256) ||
		value.EpisodeID == "" || !strings.HasPrefix(value.ReconciliationID, "cognition_reconciliation_") ||
		!strings.HasPrefix(value.PolicyCallID, "cognition_call_") || value.CallOrdinal == 0 || value.CallOrdinal > math.MaxInt64 ||
		!cognitionDigestPattern.MatchString(value.SnapshotSHA256) || !cognitionDigestPattern.MatchString(value.DecisionSHA256) ||
		value.ProposalIndex < 0 || value.ProposalIndex >= cognition.MaxLedgerProposals || !ok ||
		value.SourceKind != wantSource || value.PreProposalLedgerVersion == 0 || value.PreProposalLedgerVersion > math.MaxInt64 ||
		!cognitionDigestPattern.MatchString(value.PreProposalLedgerSHA256) ||
		value.ReplayDescriptor.SourceKind != value.SourceKind || value.ReplayDescriptor.LedgerSHA256 != value.PreProposalLedgerSHA256 ||
		value.ReplayDescriptor.ExpectedVersion != value.PreProposalLedgerVersion+uint64(value.ProposalIndex) ||
		value.ReplayDescriptor.Actor != taskstate.AuthorityModelProposal || value.ReplayDescriptor.EntryID != value.Command.ID ||
		value.ReplayDescriptor.CommandID != value.Command.CommandID || err != nil || descriptor.Actor != taskstate.AuthorityModelProposal ||
		descriptor.Kind != taskstate.CommandAddEntry || descriptor.SHA256 != value.ReplayDescriptor.CommandSHA256 ||
		value.Command.Kind != wantEntry || value.EntryURI != cognitionProposalEntryURI(value.ReplayDescriptor.LedgerID, value.Command.ID) ||
		value.OutputLedgerVersion != value.Command.ExpectedVersion+1 || value.OutputLedgerVersion > math.MaxInt64 ||
		value.OutputLedgerStatus != taskstate.LedgerActive || value.Command.Metadata.Validate() != nil {
		return cognitionProposalMaterializationTupleError(value, wantSource, wantEntry, descriptor, err, ok)
	}
	if err := value.ReplayDescriptor.Validate(value.Command); err != nil {
		return fmt.Errorf("%w: proposal replay descriptor: %v", ErrCognitionConflict, err)
	}
	payload, err := exactjson.Canonical(value)
	if err != nil || len(payload) > MaxCognitionTracePayloadBytes {
		return fmt.Errorf(
			"%w: proposal materialization exceeds the hard trace cap: %v",
			ErrCognitionConflict, err,
		)
	}
	return nil
}

// VerifyCognitionProposalMaterializationTrace proves one trace member. Callers
// requiring reconciliation-wide totality must use the set verifier.
func VerifyCognitionProposalMaterializationTrace(
	value CognitionProposalMaterialization,
	authority CognitionProposalMaterializationTraceAuthority,
	snapshot cognition.RuntimeSnapshot,
	decision cognition.CognitionDecision,
	actionSchema cognition.ActionSchema,
) error {
	if err := authority.validateFor(value); err != nil {
		return err
	}
	return VerifyCognitionProposalMaterialization(value, cognitionstate.ModelProposalInput{
		Ledger: value.PreProposalLedger, ScopeNodeID: taskstate.NodeID(decision.ObligationID),
		Snapshot: snapshot, Decision: decision, ActionSchema: actionSchema,
	})
}

func VerifyCognitionProposalMaterialization(
	value CognitionProposalMaterialization,
	input cognitionstate.ModelProposalInput,
) error {
	if err := value.Validate(); err != nil {
		return err
	}
	decisionSHA, err := cognitionruntime.DecisionSHA256(input.Decision)
	ledgerSHA, ledgerErr := cognitionProposalLedgerSHA(input.Ledger)
	if err != nil || ledgerErr != nil || value.SnapshotSHA256 != input.Snapshot.SHA256() ||
		value.DecisionSHA256 != decisionSHA || value.EpisodeID != input.Snapshot.CurrentRevision().EpisodeID ||
		value.PreProposalLedgerVersion != input.Ledger.Version || value.PreProposalLedgerSHA256 != ledgerSHA ||
		value.ProposalIndex >= len(input.Decision.Proposals) ||
		!reflect.DeepEqual(value.Proposal, input.Decision.Proposals[value.ProposalIndex]) {
		return fmt.Errorf("%w: proposal materialization source authority changed", ErrCognitionConflict)
	}
	mutations, err := cognitionstate.MapModelProposals(input)
	if err != nil {
		return fmt.Errorf("%w: rederive proposal materialization: %v", ErrCognitionConflict, err)
	}
	for _, mutation := range mutations {
		if mutation.Descriptor().ExpectedVersion != input.Ledger.Version+uint64(value.ProposalIndex) {
			continue
		}
		if mutation.Descriptor() != value.ReplayDescriptor || !reflect.DeepEqual(mutation.Command(), value.Command) {
			return fmt.Errorf("%w: proposal materialization differs from MapModelProposals", ErrCognitionConflict)
		}
		return nil
	}
	return fmt.Errorf("%w: proposal materialization has no rederived mutation", ErrCognitionConflict)
}

func DecodeCognitionProposalMaterialization(
	raw []byte, payloadSHA256 string,
) (CognitionProposalMaterialization, error) {
	var value CognitionProposalMaterialization
	if len(raw) > MaxCognitionTracePayloadBytes {
		return value, fmt.Errorf(
			"%w: proposal materialization exceeds the hard trace cap",
			ErrCognitionConflict,
		)
	}
	if !cognitionDigestPattern.MatchString(payloadSHA256) ||
		exactjson.ValidateObject(raw, &value, "cognition proposal materialization") != nil ||
		json.Unmarshal(raw, &value) != nil || value.Validate() != nil {
		return value, fmt.Errorf("%w: proposal materialization payload is invalid", ErrCognitionConflict)
	}
	canonical, err := exactjson.Canonical(value)
	if err != nil || !bytes.Equal(raw, canonical) || cognitionPayloadSHA(raw) != payloadSHA256 {
		return value, fmt.Errorf("%w: proposal materialization payload changed", ErrCognitionConflict)
	}
	return value, nil
}

func cognitionProposalMaterializationKinds(
	kind cognition.LedgerProposalKind,
) (cognitionstate.SourceKind, taskstate.EntryKind, bool) {
	switch kind {
	case cognition.ProposalObservation:
		return cognitionstate.SourceModelObservation, taskstate.EntryObservation, true
	case cognition.ProposalHypothesis:
		return cognitionstate.SourceModelHypothesis, taskstate.EntryHypothesis, true
	case cognition.ProposalQuestion:
		return cognitionstate.SourceModelQuestion, taskstate.EntryQuestion, true
	case cognition.ProposalObligation:
		return cognitionstate.SourceModelObligation, taskstate.EntryDecisionCandidate, true
	case cognition.ProposalPlanRevision:
		return cognitionstate.SourceModelPlanRevision, taskstate.EntryDecisionCandidate, true
	default:
		return "", "", false
	}
}

func cognitionProposalEntryURI(ledgerID taskstate.LedgerID, entryID taskstate.EntryID) string {
	return "task:ledger/" + string(ledgerID) + "/entry/" + string(entryID)
}

func cognitionProposalCanonicalSHA(value any) (string, error) {
	raw, err := exactjson.Canonical(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func cognitionProposalLedgerSHA(value taskstate.MaterializedState) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func cognitionProposalLedgerJSONSHA(value taskstate.MaterializedState) (string, error) {
	raw, err := exactjson.Canonical(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
