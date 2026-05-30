package projectdebugger

type BugTicket struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	Column      string   `json:"column"`
	Checklist   []string `json:"checklist"`
	RefFiles    []string `json:"ref_files"`
	Tags        []string `json:"tags"`
}

type ScanResponse struct {
	Summary     string      `json:"summary"`
	BugTickets  []BugTicket `json:"bug_tickets"`
	Suggestions []string    `json:"suggestions"`
}

type CreatedCard struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Severity string `json:"severity,omitempty"`
}

type LastRun struct {
	JobID         int64         `json:"job_id"`
	ProjectID     int64         `json:"project_id"`
	AgentSystem   string        `json:"agent_system"`
	Model         string        `json:"model"`
	Status        string        `json:"status"`
	Summary       string        `json:"summary"`
	FindingsCount int           `json:"findings_count"`
	CardsCreated  []CreatedCard `json:"cards_created"`
	Suggestions   []string      `json:"suggestions"`
	StartedAt     string        `json:"started_at"`
	CompletedAt   string        `json:"completed_at,omitempty"`
	Error         string        `json:"error,omitempty"`
}

const SettingsKey = "debugger_last_run"

type BoardCard struct {
	Title       string
	Column      string
	Description string
	PlayState   string
	Tags        []string
}

type Input struct {
	ProjectName        string
	ProjectLocation    string
	ProjectState       string
	ProjectDescription string
	AgentSystem        string
	Model              string
	MapPayload         map[string]any
	BoardCards         []BoardCard
}
