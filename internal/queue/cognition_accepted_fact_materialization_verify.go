package queue

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	CognitionAcceptedFactMaterializationInitialTracePhase = 11
	CognitionAcceptedFactMaterializationActionTracePhase  = 54
)

type CognitionAcceptedFactMaterializationTraceAuthority struct {
	TransitionID     string `json:"transition_id"`
	TransitionSHA256 string `json:"transition_sha256"`
	CallOrdinal      uint64 `json:"call_ordinal"`
	Phase            int    `json:"phase"`
	Sequence         int64  `json:"sequence"`
	ID               string `json:"id"`
	SHA256           string `json:"sha256"`
}

// VerifyCognitionAcceptedFactMaterializationTrace proves one complete,
// zero-capable transition batch and its exact terminal-trace tuple.
func VerifyCognitionAcceptedFactMaterializationTrace(
	value CognitionAcceptedFactMaterialization,
	trace CognitionAcceptedFactMaterializationTraceAuthority,
	transition cognition.Transition,
	authority cognitionstate.FactAcceptanceAuthority,
) error {
	if err := value.Validate(); err != nil {
		return err
	}
	wantPhase := CognitionAcceptedFactMaterializationActionTracePhase
	if value.CallOrdinal == 0 {
		wantPhase = CognitionAcceptedFactMaterializationInitialTracePhase
	}
	if trace.TransitionID != value.TransitionID ||
		trace.TransitionSHA256 != value.TransitionSHA256 ||
		trace.CallOrdinal != value.CallOrdinal || trace.Phase != wantPhase ||
		trace.Sequence != int64(value.TransitionRevision) || trace.ID != value.ID {
		return fmt.Errorf("%w: accepted-fact materialization trace tuple changed", ErrCognitionConflict)
	}
	payload, err := exactjson.Canonical(value)
	if err != nil || trace.SHA256 != cognitionPayloadSHA(payload) {
		return fmt.Errorf("%w: accepted-fact materialization trace payload changed", ErrCognitionConflict)
	}
	return VerifyCognitionAcceptedFactMaterialization(value, transition, authority)
}

func VerifyCognitionAcceptedFactMaterialization(
	value CognitionAcceptedFactMaterialization,
	transition cognition.Transition,
	authority cognitionstate.FactAcceptanceAuthority,
) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if err := authority.Validate(); err != nil {
		return fmt.Errorf("%w: accepted-fact executable authority: %v", ErrCognitionConflict, err)
	}
	_, transitionSHA, err := cognitionJSON(transition)
	if err != nil || transitionSHA != value.TransitionSHA256 ||
		transition.Current.EpisodeID != value.EpisodeID ||
		transition.Current.Number != value.TransitionRevision ||
		transition.ActionID != value.ActionID ||
		!reflect.DeepEqual(authority.Reference(), value.FactAuthority) {
		return fmt.Errorf("%w: accepted-fact materialization source changed", ErrCognitionConflict)
	}
	want, err := newCognitionAcceptedFactMaterialization(
		value.EpisodeID, value.ScopeObligationID, transition, authority,
		value.PreFactLedger, value.CallOrdinal,
	)
	if err != nil {
		return fmt.Errorf("%w: rederive accepted-fact materialization: %v", ErrCognitionConflict, err)
	}
	if !acceptedFactMaterializationsEqual(want, value) {
		return fmt.Errorf("%w: accepted-fact materialization differs from MapTransitionFacts", ErrCognitionConflict)
	}
	ledger, err := taskstate.RestoreLedger(value.PreFactLedger)
	if err != nil {
		return fmt.Errorf("%w: accepted-fact pre-ledger: %v", ErrCognitionConflict, err)
	}
	for index, member := range value.Members {
		event, err := ledger.Apply(member.Command)
		if err != nil || event.Version != member.OutputLedgerVersion {
			return fmt.Errorf("%w: accepted-fact member %d does not apply exactly", ErrCognitionConflict, index)
		}
	}
	state := ledger.MaterializedState()
	if state.Version != value.OutputLedgerVersion || state.Status != value.OutputLedgerStatus {
		return fmt.Errorf("%w: accepted-fact materialization final ledger changed", ErrCognitionConflict)
	}
	return nil
}

func cognitionAcceptedFactMemberEqual(
	left, right CognitionAcceptedFactMaterializationMember,
) bool {
	return reflect.DeepEqual(left, right)
}
