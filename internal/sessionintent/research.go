package sessionintent

import (
	"encoding/json"

	"github.com/gryph/omnidex/internal/model"
)

// ResearchHistoryPageSize bounds each retained-history response.
const ResearchHistoryPageSize = 40

// Research is a separate conversation, never an execution or ledger authority.
type ResearchRequest struct {
	OperationID       model.LifecycleOperationID `json:"operation_id"`
	WorkspaceIdentity string                     `json:"workspace_identity"`
	ConversationID    model.LifecycleOperationID `json:"conversation_id,omitempty"`
	Fork              *ResearchSeed              `json:"fork,omitempty"`
	Text              string                     `json:"text"`
}

type ResearchTurn struct {
	OperationID    model.LifecycleOperationID `json:"operation_id"`
	ConversationID model.LifecycleOperationID `json:"conversation_id,omitempty"`
	Fork           *ResearchSeed              `json:"fork,omitempty"`
	Question       string                     `json:"question"`
	Answer         string                     `json:"answer"`
	Error          string                     `json:"error,omitempty"`
}

type ResearchContext struct {
	State  model.ChannelSessionState `json:"state"`
	Fork   *ResearchSeed             `json:"fork,omitempty"`
	Recent []ResearchTurn            `json:"recent"`
}

// ResearchSeed records the exact owner selection and code-resolved source bytes.
// It is background context, never executable authority.
type ResearchSeed struct {
	SourceChannel  model.ChannelID   `json:"source_channel"`
	MessageIDs     []int64           `json:"message_ids,omitempty"`
	LedgerID       int64             `json:"ledger_id,omitempty"`
	LedgerRevision int64             `json:"ledger_revision,omitempty"`
	Messages       []ResearchMessage `json:"messages,omitempty"`
	Ledger         json.RawMessage   `json:"ledger,omitempty"`
}

// ResearchMessage is the source excerpt transported as background data.
type ResearchMessage struct {
	ID   int64  `json:"id"`
	Role string `json:"role"`
	Text string `json:"text"`
}
