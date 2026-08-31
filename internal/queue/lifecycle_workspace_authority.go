package queue

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

// requireLifecycleWorkspaceAuthority binds an optional CLI lifecycle command
// to the exact immutable assistant-channel job that the CLI observed. Browser
// and other non-CLI callers may omit both values; supplying either makes the
// pair authoritative.
func requireLifecycleWorkspaceAuthority(
	job model.Job,
	workspaceRoot string,
	workspaceIdentity string,
) error {
	if workspaceRoot == "" && workspaceIdentity == "" {
		return nil
	}
	if err := validateLifecycleWorkspaceBinding(workspaceRoot, workspaceIdentity); err != nil {
		return err
	}
	if job.Pipeline != model.PipelineChat {
		return fmt.Errorf(
			"%w: job %d is not one assistant channel job",
			ErrChannelSessionWorkspace,
			job.ID,
		)
	}
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		return fmt.Errorf("decode job %d lifecycle workspace authority: %w", job.ID, err)
	}
	if err := validateChannelTurnMetadata(binding); err != nil {
		return fmt.Errorf("job %d lifecycle workspace authority: %w", job.ID, err)
	}
	if binding.ChannelMode != model.ChannelModeAssistant ||
		binding.ClientCWD != workspaceRoot ||
		binding.ClientWorkspaceIdentity != workspaceIdentity {
		return fmt.Errorf(
			"%w: job %d differs from the exact CLI workspace authority",
			ErrChannelSessionWorkspace,
			job.ID,
		)
	}
	return nil
}
