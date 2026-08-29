package omnidex

import (
	"encoding/json"
	"time"
)

type DirectDataSourceInput struct {
	Name         string
	Host         string
	Port         int
	DatabaseName string
	Username     string
	Password     string
	SSLMode      string
	UseDSN       bool
	DSN          string
}

type DelegatedDataSourceInput struct {
	Name          string
	AuthorityURL  string
	CredentialEnv string
}

type DataSource struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Driver           string     `json:"driver"`
	ExecutionMode    string     `json:"execution_mode"`
	Host             string     `json:"host"`
	Port             int        `json:"port"`
	DatabaseName     string     `json:"database_name"`
	Username         string     `json:"username"`
	SSLMode          string     `json:"ssl_mode"`
	UseDSN           bool       `json:"use_dsn"`
	AuthorityURL     string     `json:"authority_url"`
	CredentialEnv    string     `json:"credential_env"`
	ReadOnly         bool       `json:"read_only"`
	PasswordSet      bool       `json:"password_set"`
	PasswordHint     string     `json:"password_hint"`
	LastTestStatus   string     `json:"last_test_status"`
	LastTestMessage  string     `json:"last_test_message"`
	LastTestAt       *time.Time `json:"last_test_at,omitempty"`
	CatalogUpdatedAt *time.Time `json:"catalog_updated_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type CreateChannelInput struct {
	ID            string
	Name          string
	Tags          []string
	WorkspaceRoot string
	DataSourceID  string
}

type Channel struct {
	ID            string    `json:"id"`
	Scope         string    `json:"scope"`
	Name          string    `json:"name"`
	Tags          []string  `json:"tags"`
	ProjectID     int64     `json:"project_id"`
	WorkspaceRoot string    `json:"workspace_root"`
	DataSourceID  string    `json:"data_source_id,omitempty"`
	Mode          string    `json:"mode"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ChannelMessage struct {
	ID        int64     `json:"id"`
	ChannelID string    `json:"channel_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type SendMessageInput struct {
	Prompt                   string
	DelegatedDataAuthorityID string
}

type SendMessageResult struct {
	Channel     Channel        `json:"channel"`
	UserMessage ChannelMessage `json:"user_message"`
	Job         Job            `json:"job"`
}

type MessagePage struct {
	ChannelID    string           `json:"channel_id"`
	Messages     []ChannelMessage `json:"messages"`
	NextBeforeID *int64           `json:"next_before_id"`
	HasMore      bool             `json:"has_more"`
}

type Job struct {
	ID                int64           `json:"id"`
	Instruction       string          `json:"instruction"`
	Pipeline          string          `json:"pipeline"`
	Status            string          `json:"status"`
	Result            string          `json:"result,omitempty"`
	Error             string          `json:"error,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	CurrentGeneration int64           `json:"current_generation"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
}

type JobStep struct {
	ID                     int64      `json:"id"`
	JobID                  int64      `json:"job_id"`
	Action                 string     `json:"action"`
	SortIndex              int        `json:"sort_index"`
	Status                 string     `json:"status"`
	Generation             int64      `json:"generation"`
	SupersededAtGeneration *int64     `json:"superseded_at_generation,omitempty"`
	WorkerID               string     `json:"worker_id,omitempty"`
	Output                 string     `json:"output,omitempty"`
	Error                  string     `json:"error,omitempty"`
	StartedAt              *time.Time `json:"started_at,omitempty"`
	FinishedAt             *time.Time `json:"finished_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type JobContext struct {
	ID        int64     `json:"id"`
	StepID    int64     `json:"step_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

type JobDetails struct {
	Job      Job          `json:"job"`
	Steps    []JobStep    `json:"steps"`
	Contexts []JobContext `json:"contexts"`
}
