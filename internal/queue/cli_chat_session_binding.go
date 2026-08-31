package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

// requireCLIChatSessionWorkspaceBinding preserves generic assistant-channel
// behavior while making the reserved CLI channel identity inseparable from
// the exact physical workspace identity which derived it.
func requireCLIChatSessionWorkspaceBinding(
	channelID model.ChannelID,
	workspaceRoot string,
	workspaceIdentity string,
) error {
	if !projectroot.IsCLIChatChannelID(channelID) {
		return nil
	}
	expectedID, err := projectroot.CLIChatChannelID(workspaceRoot, workspaceIdentity)
	if err != nil {
		return fmt.Errorf("validate CLI channel workspace binding: %w", err)
	}
	if channelID != expectedID {
		return fmt.Errorf(
			"%w: CLI channel %q differs from the exact workspace identity binding",
			ErrChannelSessionWorkspace,
			channelID,
		)
	}
	return nil
}
