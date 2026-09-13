package queue

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

// requireCLIChatSessionWorkspaceBinding compares the retained physical directory
// identity directly. Ordinary assistant channels have no CLI-specific binding.
func requireCLIChatSessionWorkspaceBinding(
	channelID model.ChannelID,
	boundWorkspaceIdentity *string,
	workspaceIdentity string,
) error {
	if boundWorkspaceIdentity == nil && !strings.HasPrefix(string(channelID), projectroot.CLIChatChannelIDPrefix) {
		return nil
	}
	if !projectroot.IsCLIChatChannelID(channelID) || boundWorkspaceIdentity == nil ||
		projectroot.ValidateClientWorkspaceIdentity(*boundWorkspaceIdentity) != nil ||
		*boundWorkspaceIdentity != workspaceIdentity {
		return fmt.Errorf(
			"%w: CLI channel %q differs from the exact workspace identity binding",
			ErrChannelSessionWorkspace,
			channelID,
		)
	}
	return nil
}
