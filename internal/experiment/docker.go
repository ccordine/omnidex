package experiment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Run uses the same workspace lifecycle as multi-command production stages.
func (docker *Docker) Run(parent context.Context, request Request) (result Result, resultErr error) {
	if err := validateRequest(request); err != nil {
		return result, err
	}
	if parent == nil {
		return result, fmt.Errorf("Docker experiment requires a context")
	}
	ctx, cancel := context.WithTimeout(parent, request.Timeout)
	defer cancel()
	workspace, err := docker.Open(ctx, request.ImageID, request.Input)
	if err != nil {
		return result, err
	}
	defer func() { resultErr = errors.Join(resultErr, workspace.Close()) }()
	result, err = workspace.Run(ctx, Command{Argv: request.Argv, Environment: request.Environment, Stdin: request.Stdin, Timeout: request.Timeout})
	if err != nil {
		return result, err
	}
	if len(request.Collect) != 0 {
		result.Files, err = workspace.Collect(ctx, request.Collect)
	}
	return result, err
}

func (docker *Docker) create(ctx context.Context, request Request, networkMode string) (_ *Workspace, resultErr error) {
	if networkMode != "none" && networkMode != "bridge" {
		return nil, fmt.Errorf("Docker creation requires a registered network boundary")
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, fmt.Errorf("allocate Docker experiment identity: %w", err)
	}
	name := "omnidex-experiment-" + hex.EncodeToString(entropy[:])
	workspace := &Workspace{docker: docker, name: name, imageID: request.ImageID, networkMode: networkMode, acquiring: networkMode == "bridge"}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, workspace.Close())
		}
	}()
	argv := []string{
		"container", "create", "--name", name, "--label", "com.omnidex.experiment=" + name,
		"--pull", "never", "--network", networkMode, "--user", "65532:65532",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--init", "--pids-limit", "256", "--memory", "2g", "--memory-swap", "2g",
		"--interactive", "--log-driver", "none", "--workdir", WorkingDirectory, "--entrypoint", request.Argv[0],
	}
	argv = append(argv, request.ImageID)
	argv = append(argv, request.Argv[1:]...)
	stdout, stderr, err := docker.capture(ctx, argv, nil)
	if err != nil {
		return nil, dockerOperationError("create", stderr, err)
	}
	id := strings.TrimSpace(string(stdout))
	if !containerIDPattern.MatchString(id) {
		return nil, fmt.Errorf("Docker create returned no exact container ID")
	}
	inspection, err := docker.inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateCreatedContainer(inspection, request, name, networkMode); err != nil {
		return nil, err
	}
	workspace.containerID = id
	workspace.environment = append([]string(nil), inspection.Config.Env...)
	if err := docker.copyInput(ctx, id, request.Input); err != nil {
		return nil, err
	}
	return workspace, nil
}

func (docker *Docker) cleanup(ctx context.Context, name, id string) error {
	filter := "label=com.omnidex.experiment=" + name
	if id != "" {
		filter = "id=" + id
	}
	list := []string{"container", "ls", "--all", "--quiet", "--no-trunc", "--filter", filter}
	if id == "" {
		list = append(list, "--filter", "name=^/"+name+"$")
	}
	stdout, stderr, err := docker.capture(ctx, list, nil)
	if err != nil {
		return dockerOperationError("observe cleanup ownership", stderr, err)
	}
	ids := strings.Fields(string(stdout))
	if len(ids) == 0 {
		return nil
	}
	if len(ids) != 1 || !containerIDPattern.MatchString(ids[0]) || (id != "" && ids[0] != id) {
		return fmt.Errorf("Docker experiment cleanup did not resolve one exact owned container")
	}
	_, stderr, removeErr := docker.capture(ctx, []string{"container", "rm", "--force", "--volumes", ids[0]}, nil)
	stdout, observationStderr, err := docker.capture(ctx, list, nil)
	if err != nil {
		return errors.Join(dockerOperationError("remove", stderr, removeErr), dockerOperationError("verify removal", observationStderr, err))
	}
	if len(strings.Fields(string(stdout))) != 0 {
		return errors.Join(fmt.Errorf("Docker experiment container %s remains after cleanup", ids[0]), dockerOperationError("remove", stderr, removeErr))
	}
	return nil
}
