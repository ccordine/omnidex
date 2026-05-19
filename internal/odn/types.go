package odn

import "fmt"

const (
	sessionVersion                 = "1.0"
	intentConfidenceThreshold      = 0.70
	defaultProjectFolderName       = "test-go-html"
	defaultOllamaEndpoint          = "http://localhost:11434/api/chat"
	defaultOllamaModel             = "qwen2.5-coder:7b"
	maxConversationHistoryMessages = 16
)

type PermissionMode string

const (
	PermissionAsk  PermissionMode = "ask_permission"
	PermissionFull PermissionMode = "full_access"
)

func ParsePermissionMode(raw string) (PermissionMode, error) {
	switch PermissionMode(raw) {
	case PermissionAsk, PermissionFull:
		return PermissionMode(raw), nil
	default:
		return "", fmt.Errorf("invalid permission mode %q", raw)
	}
}

type IntentClassification string

const (
	IntentConversation IntentClassification = "conversation_mode"
	IntentExecution    IntentClassification = "execution_mode"
	IntentAmbiguous    IntentClassification = "ambiguous"
)

type Message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type Event struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Summary   string            `json:"summary"`
	Details   map[string]string `json:"details,omitempty"`
	CreatedAt string            `json:"created_at"`
}

type Turn struct {
	ID                   string               `json:"id"`
	UserInput            string               `json:"user_input"`
	IntentClassification IntentClassification `json:"intent_classification"`
	Confidence           float64              `json:"confidence"`
	ReasonCodes          []string             `json:"reason_codes"`
	Response             string               `json:"response"`
	Events               []Event              `json:"events"`
	CreatedAt            string               `json:"created_at"`
}

type Session struct {
	Version       string         `json:"version"`
	WorkspacePath string         `json:"workspace_path"`
	WorkspaceHash string         `json:"workspace_hash"`
	Permission    PermissionMode `json:"permission_mode"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	Messages      []Message      `json:"messages"`
	Turns         []Turn         `json:"turns"`
}

type IntentResult struct {
	Classification IntentClassification
	Confidence     float64
	ReasonCodes    []string
}

type PlannedAction struct {
	ID          string
	Kind        string
	Description string
	Path        string
	Content     string
	RiskTier    int
}

type ExecutionPlan struct {
	Name    string
	Summary string
	Actions []PlannedAction
}
