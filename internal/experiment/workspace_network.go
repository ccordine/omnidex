package experiment

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (workspace *Workspace) Run(ctx context.Context, command Command) (Result, error) {
	return workspace.run(ctx, command, false)
}

// Acquire is available only during the initial dependency acquisition interval.
// The constructor supplies that interval's exact inputs. Source transfer and
// ordinary execution remain unavailable until SealNetwork observes detachment.
func (workspace *Workspace) Acquire(ctx context.Context, command Command) (Result, error) {
	return workspace.run(ctx, command, true)
}

func (workspace *Workspace) SealNetwork(parent context.Context) error {
	if workspace == nil || parent == nil {
		return fmt.Errorf("network sealing requires a workspace and context")
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	if err := workspace.requireRunning(ctx); err != nil {
		return errors.Join(err, workspace.close())
	}
	if !workspace.acquiring {
		return nil
	}
	_, stderr, err := workspace.docker.capture(ctx, []string{"network", "disconnect", "bridge", workspace.containerID}, nil)
	if err != nil {
		return errors.Join(dockerOperationError("seal acquisition network", stderr, err), workspace.close())
	}
	workspace.acquiring = false
	if err := workspace.requireRunning(ctx); err != nil {
		return errors.Join(err, workspace.close())
	}
	return nil
}

func (workspace *Workspace) validateNetwork(inspection containerInspection) error {
	networks := *inspection.NetworkSettings.Networks
	if workspace.networkMode == "none" && !workspace.acquiring {
		if _, exists := networks["none"]; exists && len(networks) == 1 {
			return nil
		}
	} else if workspace.networkMode == "bridge" {
		if workspace.acquiring {
			if _, exists := networks["bridge"]; exists && len(networks) == 1 {
				return nil
			}
		} else if len(networks) == 0 {
			return nil
		}
	}
	return fmt.Errorf("Docker experiment network attachments differ from their code-owned boundary")
}
