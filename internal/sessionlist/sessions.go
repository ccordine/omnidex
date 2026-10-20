package sessionlist

import (
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"time"
)

// Entry is a discovery projection, never workspace or operation authority.
type Entry struct {
	Connected         bool            `json:"connected"`
	ID                model.ChannelID `json:"id"`
	Directory         string          `json:"directory"`
	WorkspaceIdentity string          `json:"workspace_identity"`
	Activity          string          `json:"activity"`
	ObservedAt        time.Time       `json:"observed_at"`
}

const PageSize = 100

type Listing struct {
	NextCursor model.ChannelID `json:"next_cursor,omitempty"`
	Sessions   []Entry         `json:"sessions"`
}

func (entry Entry) Validate(clientID string) error {
	owner, _, err := projectroot.ParseClientWorkspaceIdentity(entry.WorkspaceIdentity)
	if err != nil || owner != clientID {
		return fmt.Errorf("session differs from client installation")
	}
	if !projectroot.IsCLIChatChannelID(entry.ID) {
		return fmt.Errorf("invalid CLI session identity")
	}
	if err := model.ValidateChannelWorkspaceRoot(entry.Directory); err != nil {
		return err
	}
	if entry.ObservedAt.IsZero() {
		return fmt.Errorf("session observation has no timestamp")
	}
	switch entry.Activity {
	case "idle", "pending", "running", "waiting_input":
	default:
		return fmt.Errorf("invalid observed session activity %q", entry.Activity)
	}
	return nil
}
