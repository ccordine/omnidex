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
	if err := model.ValidateChannelWorkspaceRoot(value); err != nil {
		return err
	}
	root.Value = value
	root.Present = true
	return nil
}

func (s *Server) resolveChannelCreateWorkspaceRoot(requested channelCreateWorkspaceRoot) (string, error) {
	if requested.Present {
		return requested.Value, nil
	}
	configured := s.providerConfig.WorkspaceRoot
	if err := model.ValidateChannelWorkspaceRoot(configured); err != nil {
		return "", fmt.Errorf("server default channel workspace root is invalid: %w", err)
	}
	return configured, nil
}
