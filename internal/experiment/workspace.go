package experiment

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Workspace retains source, dependencies, and compiler output inside one private
// container. Commands are serialized; a transport failure destroys that container.
type Workspace struct {
	mu                                     sync.Mutex
	docker                                 *Docker
	name, containerID, imageID, apiVersion string
	environment                            []string
	closed                                 bool
	networkMode                            string
	acquiring                              bool
}

type Command struct {
	Argv, Environment []string
	Stdin             []byte
	Timeout           time.Duration
}

var apiVersionPattern = regexp.MustCompile(`^1\.[0-9]{2,3}$`)

func (docker *Docker) Open(parent context.Context, imageID string, input []File) (*Workspace, error) {
	return docker.open(parent, imageID, input, "none")
}

func (docker *Docker) OpenAcquisition(parent context.Context, imageID string, input []File) (*Workspace, error) {
	return docker.open(parent, imageID, input, "bridge")
}

func (docker *Docker) open(parent context.Context, imageID string, input []File, networkMode string) (_ *Workspace, resultErr error) {
	if docker == nil || docker.cli == nil || parent == nil {
		return nil, fmt.Errorf("Docker workspace requires a context and initialized client")
	}
	request := Request{ImageID: imageID, Input: input, Argv: []string{"/bin/sh", "-c", "exec sleep 2147483647"}, Timeout: 2 * time.Minute}
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, request.Timeout)
	defer cancel()
	workspace, err := docker.create(ctx, request, networkMode)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, workspace.Close())
		}
	}()
	stdout, stderr, err := docker.capture(ctx, []string{"version", "--format", "{{.Server.APIVersion}}"}, nil)
	if err != nil {
		return nil, dockerOperationError("observe Engine API", stderr, err)
	}
	workspace.apiVersion = strings.TrimSpace(string(stdout))
	var minor int
	if !apiVersionPattern.MatchString(workspace.apiVersion) {
		return nil, fmt.Errorf("Docker did not return one exact Engine API version")
	}
	_, _ = fmt.Sscanf(workspace.apiVersion, "1.%d", &minor)
	if minor < 35 {
		return nil, fmt.Errorf("Docker Engine API %s lacks explicit exec working directories", workspace.apiVersion)
	}
	stdout, stderr, err = docker.capture(ctx, []string{"container", "start", workspace.containerID}, nil)
	if err != nil {
		return nil, dockerOperationError("start workspace", stderr, err)
	}
	if strings.TrimSpace(string(stdout)) != workspace.containerID {
		return nil, fmt.Errorf("Docker started a different workspace identity")
	}
	if err := workspace.requireRunning(ctx); err != nil {
		return nil, err
	}
	return workspace, nil
}

func (workspace *Workspace) requireRunning(ctx context.Context) error {
	if workspace.closed {
		return fmt.Errorf("Docker experiment workspace is closed")
	}
	inspection, err := workspace.docker.inspect(ctx, workspace.containerID)
	if err != nil {
		return err
	}
	if inspection.Image != workspace.imageID || inspection.Name != "/"+workspace.name ||
		inspection.Config.Labels["com.omnidex.experiment"] != workspace.name ||
		inspection.State.Status != "running" || !*inspection.State.Running || *inspection.State.OOMKilled ||
		inspection.HostConfig.NetworkMode != workspace.networkMode || len(*inspection.Mounts) != 0 {
		return fmt.Errorf("Docker experiment lost its exact running workspace authority")
	}
	return workspace.validateNetwork(inspection)
}

func (workspace *Workspace) Close() error {
	if workspace == nil {
		return nil
	}
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	return workspace.close()
}

func (workspace *Workspace) close() error {
	if workspace.closed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := workspace.docker.cleanup(ctx, workspace.name, workspace.containerID); err != nil {
		return err
	}
	workspace.closed = true
	return nil
}

func (workspace *Workspace) Collect(ctx context.Context, paths []string) ([]File, error) {
	if workspace == nil || ctx == nil {
		return nil, fmt.Errorf("Docker collection requires a workspace and context")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	if err := validateRequest(Request{ImageID: workspace.imageID, Argv: []string{"collect"}, Collect: paths, Timeout: time.Minute}); err != nil {
		return nil, err
	}
	if err := workspace.requireRunning(ctx); err != nil {
		return nil, err
	}
	return workspace.docker.collect(ctx, workspace.containerID, paths, "")
}
