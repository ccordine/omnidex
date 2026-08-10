package cognitiongauntlet

import "fmt"

type FailureClass string

const (
	FailureAcquisition     FailureClass = "acquisition_retrieval"
	FailureStateRecording  FailureClass = "state_recording"
	FailureRetention       FailureClass = "retention"
	FailureProjection      FailureClass = "context_projection"
	FailureModelPolicy     FailureClass = "model_policy"
	FailureContractRuntime FailureClass = "contract_runtime"
	FailureStaleState      FailureClass = "stale_state"
	FailureContinuity      FailureClass = "continuity"
	FailureCompletion      FailureClass = "completion"
	FailureResourceBudget  FailureClass = "resource_budget"
	FailureUnattributed    FailureClass = "unattributed"
)

type FailureTrace struct {
	NecessaryEvidence    bool
	Acquired             bool
	Recorded             bool
	ReleasedBeforeUse    bool
	ResidentAtDecision   bool
	ProjectedAtDecision  bool
	ActionSupported      bool
	ValidatorRejected    bool
	ObsoleteRevisionUsed bool
	RestartMismatch      bool
	GoalPredicateTrue    bool
	TerminalRecorded     bool
	PolicyRejected       bool
	BudgetExhausted      bool
	RequiredEvidenceID   string
	AcquisitionActionID  string
	EvidenceEntryID      string
	ReleaseEventID       string
	ProjectionID         string
	DecisionID           string
	ActionID             string
	ValidatorEventID     string
	RestartEventID       string
	CompletionCheckID    string
	PolicyFailureEventID string
	BudgetEventID        string
}

type FailureAttribution struct {
	Class     FailureClass `json:"class"`
	TraceRefs []string     `json:"trace_refs"`
}

func (attribution FailureAttribution) Validate() error {
	if !validFailureClass(attribution.Class) {
		return fmt.Errorf("cognition failure attribution class is invalid")
	}
	if attribution.TraceRefs == nil {
		return fmt.Errorf("cognition failure attribution references must be explicit")
	}
	if attribution.Class == FailureUnattributed {
		if len(attribution.TraceRefs) != 0 {
			return fmt.Errorf("unattributed cognition failure cannot claim trace proof")
		}
		return nil
	}
	if len(attribution.TraceRefs) == 0 || len(attribution.TraceRefs) > 16 {
		return fmt.Errorf("cognition failure attribution requires bounded trace proof")
	}
	seen := make(map[string]struct{}, len(attribution.TraceRefs))
	for index, ref := range attribution.TraceRefs {
		if err := requireExact(ref, fmt.Sprintf("attribution reference %d", index+1), 512); err != nil {
			return err
		}
		if _, duplicate := seen[ref]; duplicate {
			return fmt.Errorf("cognition failure attribution reference %q is duplicated", ref)
		}
		seen[ref] = struct{}{}
	}
	return nil
}

func AttributeFailure(trace FailureTrace) (FailureAttribution, error) {
	switch {
	case trace.RestartMismatch:
		return attributed(FailureContinuity, trace.RestartEventID)
	case trace.ObsoleteRevisionUsed:
		return attributed(FailureStaleState, trace.ActionID)
	case trace.ActionSupported && trace.ValidatorRejected:
		return attributed(FailureContractRuntime, trace.ActionID, trace.ValidatorEventID)
	case trace.GoalPredicateTrue && !trace.TerminalRecorded:
		return attributed(FailureCompletion, trace.CompletionCheckID)
	case trace.PolicyRejected:
		return attributed(FailureModelPolicy, trace.PolicyFailureEventID)
	case trace.BudgetExhausted:
		return attributed(FailureResourceBudget, trace.BudgetEventID)
	case trace.NecessaryEvidence && !trace.Acquired:
		return attributed(FailureAcquisition, trace.RequiredEvidenceID)
	case trace.NecessaryEvidence && trace.Acquired && !trace.Recorded:
		return attributed(FailureStateRecording, trace.RequiredEvidenceID, trace.AcquisitionActionID)
	case trace.NecessaryEvidence && trace.Recorded && trace.ReleasedBeforeUse:
		return attributed(FailureRetention, trace.EvidenceEntryID, trace.ReleaseEventID, trace.DecisionID)
	case trace.NecessaryEvidence && trace.ResidentAtDecision && !trace.ProjectedAtDecision:
		return attributed(FailureProjection, trace.EvidenceEntryID, trace.ProjectionID, trace.DecisionID)
	case trace.NecessaryEvidence && trace.ProjectedAtDecision:
		return attributed(FailureModelPolicy, trace.ProjectionID, trace.DecisionID)
	default:
		return FailureAttribution{Class: FailureUnattributed, TraceRefs: []string{}}, nil
	}
}

func attributed(class FailureClass, refs ...string) (FailureAttribution, error) {
	for index, ref := range refs {
		if err := requireExact(ref, fmt.Sprintf("%s attribution reference %d", class, index+1), 512); err != nil {
			return FailureAttribution{}, err
		}
	}
	attribution := FailureAttribution{Class: class, TraceRefs: append([]string(nil), refs...)}
	return attribution, attribution.Validate()
}

func validFailureClass(class FailureClass) bool {
	switch class {
	case FailureAcquisition, FailureStateRecording, FailureRetention, FailureProjection,
		FailureModelPolicy, FailureContractRuntime, FailureStaleState, FailureContinuity,
		FailureCompletion, FailureResourceBudget, FailureUnattributed:
		return true
	default:
		return false
	}
}
