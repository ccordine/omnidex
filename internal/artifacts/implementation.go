package artifacts

const (
	ImplementationWorkKindFile         = "file"
	ImplementationWorkKindVerification = "verification"
)

const (
	ImplementationDisciplineBootstrap     = "bootstrap"
	ImplementationDisciplineDomain        = "domain"
	ImplementationDisciplineStorage       = "storage"
	ImplementationDisciplineInterface     = "interface"
	ImplementationDisciplineEntrypoint    = "entrypoint"
	ImplementationDisciplineTest          = "test"
	ImplementationDisciplineDocumentation = "documentation"
	ImplementationDisciplineVerification  = "verification"
)

const (
	ImplementationWorkStatusPending   = "pending"
	ImplementationWorkStatusRunning   = "running"
	ImplementationWorkStatusCompleted = "completed"
	ImplementationWorkStatusFailed    = "failed"
)

// ImplementationLedgerArtifact is the durable server-owned state for one
// implementation objective. A new artifact revision is appended after every
// state transition; the latest revision is authoritative.
type ImplementationLedgerArtifact struct {
	ObjectiveID        string                   `json:"objective_id"`
	Objective          string                   `json:"objective"`
	Constraints        []string                 `json:"constraints"`
	AcceptanceCriteria []string                 `json:"acceptance_criteria"`
	Revision           int                      `json:"revision"`
	Items              []ImplementationWorkItem `json:"items"`
}

// ImplementationWorkItem grants one worker authority over one complete file,
// or grants the runtime authority to execute one final verification command.
type ImplementationWorkItem struct {
	ID                 string                 `json:"id"`
	Kind               string                 `json:"kind"`
	Discipline         string                 `json:"discipline"`
	Path               string                 `json:"path,omitempty"`
	Responsibility     string                 `json:"responsibility"`
	DependsOn          []string               `json:"depends_on"`
	AcceptanceCriteria []string               `json:"acceptance_criteria"`
	Command            *ImplementationCommand `json:"command,omitempty"`
	Status             string                 `json:"status"`
	Attempts           int                    `json:"attempts"`
	RepairCycles       int                    `json:"repair_cycles,omitempty"`
	LastError          string                 `json:"last_error,omitempty"`
	ContentSHA256      string                 `json:"content_sha256,omitempty"`
	ResultSummary      string                 `json:"result_summary,omitempty"`
}

type ImplementationCommand struct {
	Program        string   `json:"program"`
	Args           []string `json:"args"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}
