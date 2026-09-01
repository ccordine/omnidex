package api

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

type channelCreateWorkspaceRoot struct {
	Value   string
	Present bool
}

func (root *channelCreateWorkspaceRoot) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return errors.New("channel workspace_root must be omitted or contain one exact workspace root")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode channel workspace_root: %w", err)
	}
	root.Value = value
	root.Present = true
	return nil
}

func (s *Server) resolveChannelCreateWorkspaceRoot(requested channelCreateWorkspaceRoot) (string, error) {
	if s == nil {
		return "", fmt.Errorf("channel workspace authority is unavailable")
	}
	if !requested.Present {
		return "", fmt.Errorf("channel workspace_root is required")
	}
	if err := model.ValidateChannelWorkspaceRoot(requested.Value); err != nil {
		return "", err
	}
	if err := s.hostDirectoryAccess.ValidateWorkspaceRoot(requested.Value); err != nil {
		return "", fmt.Errorf("channel workspace_root: %w", err)
	}
	return requested.Value, nil
}
