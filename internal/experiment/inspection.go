package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
)

type containerInspection struct {
	ID     string `json:"Id"`
	Name   string
	Image  string
	Path   string
	Args   []string
	Config *struct {
		WorkingDir string
		User       string
		Env        []string
		Labels     map[string]string
		Tty        *bool
	}
	HostConfig *struct {
		NetworkMode string
		Privileged  *bool
		CapDrop     []string
		SecurityOpt []string
	}
	Mounts          *[]json.RawMessage
	NetworkSettings *struct{ Networks *map[string]json.RawMessage }
	State           *struct {
		Status    string
		Running   *bool
		OOMKilled *bool
		ExitCode  *int
		Error     *string
	}
}

func (docker *Docker) inspect(ctx context.Context, id string) (containerInspection, error) {
	var result containerInspection
	stdout, stderr, err := docker.capture(ctx, []string{"container", "inspect", "--format", "{{json .}}", id}, nil)
	if err != nil {
		return result, dockerOperationError("inspect", stderr, err)
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		return result, fmt.Errorf("decode exact Docker inspection: %w", err)
	}
	if result.ID != id || !imageIDPattern.MatchString(result.Image) || result.State == nil ||
		result.State.Running == nil || result.State.OOMKilled == nil || result.State.ExitCode == nil || result.State.Error == nil ||
		result.Config == nil || result.Config.Tty == nil || result.HostConfig == nil || result.HostConfig.Privileged == nil || result.Mounts == nil || result.NetworkSettings == nil || result.NetworkSettings.Networks == nil {
		return result, fmt.Errorf("Docker inspection omitted required container authority")
	}
	return result, nil
}

func validateCreatedContainer(inspection containerInspection, request Request, name, networkMode string) error {
	if inspection.Name != "/"+name || inspection.Config.Labels["com.omnidex.experiment"] != name ||
		inspection.Image != request.ImageID || inspection.Path != request.Argv[0] || !slices.Equal(inspection.Args, request.Argv[1:]) ||
		inspection.Config.WorkingDir != WorkingDirectory || inspection.Config.User != "65532:65532" || *inspection.Config.Tty ||
		inspection.HostConfig.NetworkMode != networkMode || *inspection.HostConfig.Privileged || len(*inspection.Mounts) != 0 ||
		!slices.Contains(inspection.HostConfig.CapDrop, "ALL") || !slices.Contains(inspection.HostConfig.SecurityOpt, "no-new-privileges") ||
		inspection.State.Status != "created" || *inspection.State.Running {
		return fmt.Errorf("created Docker container differs from exact experiment authority")
	}
	environment := append([]string(nil), inspection.Config.Env...)
	sort.Strings(environment)
	for _, entry := range request.Environment {
		if !slices.Contains(environment, entry) {
			return fmt.Errorf("created Docker container omitted an exact environment value")
		}
	}
	return nil
}
