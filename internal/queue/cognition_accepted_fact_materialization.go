package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	CognitionAcceptedFactMaterializationSchemaV1  = "omnidex.cognition-accepted-fact-materialization.v1"
	CognitionTraceKindAcceptedFactMaterialization = "accepted_fact_materialization"
	cognitionAcceptedFactMaterializationPrefix    = "cognition_accepted_fact_materialization_"
	MaxCognitionAcceptedFactsPerTransition        = 64
)

type CognitionAcceptedFactMaterializationMember struct {
	Index               int                        `json:"index"`
	Fact                CognitionAcceptedFactTrace `json:"fact"`
	Command             taskstate.AddEntryCommand  `json:"command"`
	EntryURI            string                     `json:"entry_uri"`
	OutputLedgerVersion uint64                     `json:"output_ledger_version"`
	OutputLedgerStatus  taskstate.LedgerStatus     `json:"output_ledger_status"`
}

type CognitionAcceptedFactMaterialization struct {
	Schema                  string                                       `json:"schema"`
	ID                      string                                       `json:"id"`
	SHA256                  string                                       `json:"sha256"`
	EpisodeID               cognition.EpisodeID                          `json:"episode_id"`
	LedgerID                taskstate.LedgerID                           `json:"ledger_id"`
	TransitionID            string                                       `json:"transition_id"`
	TransitionSHA256        string                                       `json:"transition_sha256"`
	TransitionRevision      uint64                                       `json:"transition_revision"`
	ActionID                cognition.ActionID                           `json:"action_id,omitempty"`
	CallOrdinal             uint64                                       `json:"call_ordinal"`
	ScopeObligationID       cognition.ObligationID                       `json:"scope_obligation_id"`
	FactAuthority           cognitionstate.FactAcceptanceAuthorityRef    `json:"fact_authority"`
	PreFactLedgerVersion    uint64                                       `json:"pre_fact_ledger_version"`
	PreFactLedgerSHA256     string                                       `json:"pre_fact_ledger_sha256"`
	PreFactLedgerJSONSHA256 string                                       `json:"pre_fact_ledger_json_sha256"`
	PreFactLedger           taskstate.MaterializedState                  `json:"pre_fact_ledger"`
	Members                 []CognitionAcceptedFactMaterializationMember `json:"members"`
	OutputLedgerVersion     uint64                                       `json:"output_ledger_version"`
	OutputLedgerStatus      taskstate.LedgerStatus                       `json:"output_ledger_status"`
}

type cognitionAcceptedFactMaterializationIdentity struct {
	Schema                  string                                       `json:"schema"`
	EpisodeID               cognition.EpisodeID                          `json:"episode_id"`
	LedgerID                taskstate.LedgerID                           `json:"ledger_id"`
	TransitionID            string                                       `json:"transition_id"`
	TransitionSHA256        string                                       `json:"transition_sha256"`
	TransitionRevision      uint64                                       `json:"transition_revision"`
	ActionID                cognition.ActionID                           `json:"action_id,omitempty"`
	CallOrdinal             uint64                                       `json:"call_ordinal"`
	ScopeObligationID       cognition.ObligationID                       `json:"scope_obligation_id"`
	FactAuthority           cognitionstate.FactAcceptanceAuthorityRef    `json:"fact_authority"`
	PreFactLedgerVersion    uint64                                       `json:"pre_fact_ledger_version"`
	PreFactLedgerSHA256     string                                       `json:"pre_fact_ledger_sha256"`
	PreFactLedgerJSONSHA256 string                                       `json:"pre_fact_ledger_json_sha256"`
	PreFactLedger           taskstate.MaterializedState                  `json:"pre_fact_ledger"`
	Members                 []CognitionAcceptedFactMaterializationMember `json:"members"`
	OutputLedgerVersion     uint64                                       `json:"output_ledger_version"`
	OutputLedgerStatus      taskstate.LedgerStatus                       `json:"output_ledger_status"`
}

func (value CognitionAcceptedFactMaterialization) identity() any {
	members := make([]CognitionAcceptedFactMaterializationMember, len(value.Members))
	copy(members, value.Members)
	return cognitionAcceptedFactMaterializationIdentity{
		Schema: value.Schema, EpisodeID: value.EpisodeID, LedgerID: value.LedgerID,
		TransitionID: value.TransitionID, TransitionSHA256: value.TransitionSHA256,
		TransitionRevision: value.TransitionRevision, ActionID: value.ActionID,
		CallOrdinal: value.CallOrdinal, ScopeObligationID: value.ScopeObligationID,
		FactAuthority: value.FactAuthority, PreFactLedgerVersion: value.PreFactLedgerVersion,
		PreFactLedgerSHA256:     value.PreFactLedgerSHA256,
		PreFactLedgerJSONSHA256: value.PreFactLedgerJSONSHA256,
		PreFactLedger:           value.PreFactLedger,
		Members:                 members,
		OutputLedgerVersion:     value.OutputLedgerVersion, OutputLedgerStatus: value.OutputLedgerStatus,
	}
}

func (value CognitionAcceptedFactMaterialization) Validate() error {
	identityRaw, err := exactjson.Canonical(value.identity())
	ledgerRaw, ledgerJSONErr := exactjson.Canonical(value.PreFactLedger)
	_, ledgerSHA, ledgerErr := cognitionJSON(value.PreFactLedger)
	if err != nil || value.SHA256 != cognitionPayloadSHA(identityRaw) ||
		ledgerErr != nil || ledgerJSONErr != nil ||
		value.PreFactLedgerSHA256 != ledgerSHA ||
		value.PreFactLedgerJSONSHA256 != cognitionPayloadSHA(ledgerRaw) ||
		taskstate.ValidateMaterializedState(value.PreFactLedger) != nil {
		return fmt.Errorf("%w: accepted-fact materialization identity or ledger is invalid", ErrCognitionConflict)
	}
	if value.Schema != CognitionAcceptedFactMaterializationSchemaV1 ||
		value.ID != cognitionAcceptedFactMaterializationPrefix+value.SHA256 ||
		!cognitionDigestPattern.MatchString(value.SHA256) || value.EpisodeID == "" ||
		value.LedgerID == "" || value.PreFactLedger.ID != value.LedgerID ||
		value.TransitionID != cognitionTransitionID(value.EpisodeID, value.TransitionSHA256) ||
		!cognitionDigestPattern.MatchString(value.TransitionSHA256) ||
		value.TransitionRevision == 0 || value.TransitionRevision > math.MaxInt64 ||
		value.CallOrdinal > math.MaxInt64 ||
		value.ScopeObligationID == "" || value.FactAuthority.Validate() != nil ||
		value.PreFactLedgerVersion == 0 || value.PreFactLedgerVersion > math.MaxInt64 ||
		value.PreFactLedger.Version != value.PreFactLedgerVersion || value.Members == nil ||
		len(value.Members) > MaxCognitionAcceptedFactsPerTransition ||
		value.OutputLedgerVersion != value.PreFactLedgerVersion+uint64(len(value.Members)) ||
		value.OutputLedgerVersion > math.MaxInt64 ||
		value.OutputLedgerStatus != taskstate.LedgerActive {
		return fmt.Errorf("%w: accepted-fact materialization tuple is invalid", ErrCognitionConflict)
	}
	if (value.ActionID == "") != (value.CallOrdinal == 0) ||
		(value.ActionID == "" && value.TransitionRevision != 1) {
		return fmt.Errorf("%w: accepted-fact materialization action tuple is invalid", ErrCognitionConflict)
	}
	for index, member := range value.Members {
		if err := value.validateMember(index, member); err != nil {
			return err
		}
	}
	payload, err := exactjson.Canonical(value)
	if err != nil || len(payload) > MaxCognitionTracePayloadBytes {
		return fmt.Errorf("%w: accepted-fact materialization exceeds the hard trace cap", ErrCognitionConflict)
	}
	return nil
}

func (value CognitionAcceptedFactMaterialization) validateMember(
	index int, member CognitionAcceptedFactMaterializationMember,
) error {
	descriptor, err := taskstate.DescribeCommand(member.Command)
	wantURI := "task:ledger/" + string(value.LedgerID) + "/entry/" + string(member.Command.ID)
	if member.Index != index || member.Fact.validate() != nil ||
		member.Fact.EpisodeID != value.EpisodeID || member.Fact.LedgerID != value.LedgerID ||
		member.Fact.TransitionID != value.TransitionID ||
		member.Fact.TransitionSHA256 != value.TransitionSHA256 ||
		member.Fact.ScopeObligationID != value.ScopeObligationID ||
		member.Fact.AuthoritySHA256 != value.FactAuthority.SHA256 ||
		member.Fact.Planner != value.FactAuthority.Planner ||
		member.Fact.Mapping.ExpectedVersion != value.PreFactLedgerVersion+uint64(index) ||
		member.Fact.Mapping.SourceKind != cognitionstate.SourceAcceptedFact ||
		member.Fact.Mapping.EntryID != member.Command.ID ||
		member.Fact.Mapping.CommandID != member.Command.CommandID || err != nil ||
		descriptor.Actor != taskstate.AuthorityCode || descriptor.Kind != taskstate.CommandAddEntry ||
		descriptor.SHA256 != member.Fact.Mapping.CommandSHA256 ||
		member.EntryURI != wantURI || member.Command.Kind != taskstate.EntryFact ||
		member.OutputLedgerVersion != value.PreFactLedgerVersion+uint64(index)+1 ||
		member.OutputLedgerVersion > math.MaxInt64 ||
		member.OutputLedgerStatus != taskstate.LedgerActive {
		return fmt.Errorf("%w: accepted-fact materialization member %d is invalid", ErrCognitionConflict, index)
	}
	if err := member.Fact.Mapping.Validate(member.Command); err != nil {
		return fmt.Errorf("%w: accepted-fact materialization member %d mapping: %v", ErrCognitionConflict, index, err)
	}
	return nil
}

func DecodeCognitionAcceptedFactMaterialization(
	raw []byte, payloadSHA256 string,
) (CognitionAcceptedFactMaterialization, error) {
	var value CognitionAcceptedFactMaterialization
	if len(raw) > MaxCognitionTracePayloadBytes || !cognitionDigestPattern.MatchString(payloadSHA256) ||
		exactjson.ValidateObject(raw, &value, "cognition accepted-fact materialization") != nil ||
		json.Unmarshal(raw, &value) != nil || value.Validate() != nil {
		return value, fmt.Errorf("%w: accepted-fact materialization payload is invalid", ErrCognitionConflict)
	}
	canonical, err := exactjson.Canonical(value)
	if err != nil || !bytes.Equal(raw, canonical) || cognitionPayloadSHA(raw) != payloadSHA256 {
		return value, fmt.Errorf("%w: accepted-fact materialization payload changed", ErrCognitionConflict)
	}
	return value, nil
}

func acceptedFactMaterializationsEqual(
	left, right CognitionAcceptedFactMaterialization,
) bool {
	return reflect.DeepEqual(left, right)
}

func validAcceptedFactMaterializationID(value string) bool {
	return strings.HasPrefix(value, cognitionAcceptedFactMaterializationPrefix) &&
		cognitionDigestPattern.MatchString(strings.TrimPrefix(value, cognitionAcceptedFactMaterializationPrefix))
}
