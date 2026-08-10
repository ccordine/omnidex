package cognition

const (
	ObligationGraphSchemaV1     = "omnidex.cognition-obligation-graph.v1"
	InitialObligationGeneration = uint64(1)
)

type ObligationStatus string

const (
	ObligationProposed   ObligationStatus = "proposed"
	ObligationReady      ObligationStatus = "ready"
	ObligationBlocked    ObligationStatus = "blocked"
	ObligationActive     ObligationStatus = "active"
	ObligationSatisfied  ObligationStatus = "satisfied"
	ObligationFailed     ObligationStatus = "failed"
	ObligationSuperseded ObligationStatus = "superseded"
)

type ObligationSpec struct {
	ID              ObligationID       `json:"id"`
	ParentID        ObligationID       `json:"parent_id,omitempty"`
	Desired         GoalExpression     `json:"desired"`
	DependsOn       []ObligationID     `json:"depends_on"`
	SupportingRefs  []EvidenceRef      `json:"supporting_refs"`
	CompletionCheck CompletionCheckRef `json:"completion_check"`
}

type Obligation struct {
	ID                   ObligationID       `json:"id"`
	ParentID             ObligationID       `json:"parent_id,omitempty"`
	Desired              GoalExpression     `json:"desired"`
	Status               ObligationStatus   `json:"status"`
	DependsOn            []ObligationID     `json:"depends_on"`
	SupportingRefs       []EvidenceRef      `json:"supporting_refs"`
	CompletionCheck      CompletionCheckRef `json:"completion_check"`
	Completion           *CompletionResult  `json:"completion,omitempty"`
	CreatedGeneration    uint64             `json:"created_generation"`
	SupersededGeneration uint64             `json:"superseded_generation,omitempty"`
}

func (obligation Obligation) Clone() Obligation {
	obligation.Desired = obligation.Desired.Clone()
	obligation.DependsOn = cloneSlice(obligation.DependsOn)
	obligation.SupportingRefs = cloneSlice(obligation.SupportingRefs)
	if obligation.Completion != nil {
		completion := obligation.Completion.Clone()
		obligation.Completion = &completion
	}
	return obligation
}

type ObligationGraphSnapshot struct {
	Schema      string       `json:"schema"`
	Generation  uint64       `json:"generation"`
	RootID      ObligationID `json:"root_id"`
	Obligations []Obligation `json:"obligations"`
	SHA256      string       `json:"sha256"`
}

func (snapshot ObligationGraphSnapshot) Clone() ObligationGraphSnapshot {
	snapshot.Obligations = cloneObligations(snapshot.Obligations)
	return snapshot
}

func cloneObligations(obligations []Obligation) []Obligation {
	if obligations == nil {
		return nil
	}
	cloned := make([]Obligation, len(obligations))
	for index, obligation := range obligations {
		cloned[index] = obligation.Clone()
	}
	return cloned
}

type ObligationGraphTerminalStatus string

const (
	ObligationGraphRunning   ObligationGraphTerminalStatus = "running"
	ObligationGraphSatisfied ObligationGraphTerminalStatus = "satisfied"
	ObligationGraphFailed    ObligationGraphTerminalStatus = "failed"
)

type ObligationGraph struct {
	generation uint64
	rootID     ObligationID
	items      map[ObligationID]Obligation
}
