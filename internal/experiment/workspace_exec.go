package experiment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"
)

type execInspection struct {
	ID, ContainerID string
	Running         *bool
	ExitCode        *int
	ProcessConfig   *struct {
		Entrypoint      string
		Arguments       []string
		User            string
		Tty, Privileged *bool
	}
}

func (workspace *Workspace) run(parent context.Context, command Command, acquisition bool) (result Result, resultErr error) {
	if workspace == nil || parent == nil {
		return result, fmt.Errorf("Docker command requires an active workspace and context")
	}
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	if workspace.acquiring != acquisition {
		return result, fmt.Errorf("Docker command differs from its acquisition network authority")
	}
	if err := validateRequest(Request{ImageID: workspace.imageID, Argv: command.Argv, Environment: command.Environment, Stdin: command.Stdin, Timeout: command.Timeout}); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, command.Timeout)
	defer cancel()
	result = Result{ContainerID: workspace.containerID, ImageID: workspace.imageID, Argv: append([]string(nil), command.Argv...), WorkingDirectory: WorkingDirectory, StartedAt: time.Now().UTC()}
	defer func() { result.FinishedAt = time.Now().UTC() }()
	result.Environment, resultErr = workspace.commandEnvironment(command.Environment)
	if resultErr != nil {
		return result, resultErr
	}
	if err := validateRequest(Request{ImageID: workspace.imageID, Argv: command.Argv, Environment: result.Environment, Stdin: command.Stdin, Timeout: command.Timeout}); err != nil {
		return result, err
	}
	if err := workspace.requireRunning(ctx); err != nil {
		return result, errors.Join(err, workspace.close())
	}
	networkEnabled := workspace.acquiring
	result.NetworkEnabled = &networkEnabled
	prefix := "/v" + workspace.apiVersion
	payload := map[string]any{
		"AttachStdin": true, "AttachStdout": true, "AttachStderr": true, "Tty": false,
		"Privileged": false, "User": "65532:65532", "Cmd": command.Argv,
		"Env": result.Environment, "WorkingDir": WorkingDirectory,
	}
	var created struct{ ID string }
	if err := dockerAPI(ctx, http.MethodPost, prefix+"/containers/"+workspace.containerID+"/exec", payload, &created, nil); err != nil {
		return result, errors.Join(err, workspace.close())
	}
	if !containerIDPattern.MatchString(created.ID) {
		return result, errors.Join(fmt.Errorf("Docker did not create one exact exec identity"), workspace.close())
	}
	target := prefix + "/exec/" + created.ID
	if _, err := workspace.inspectExec(ctx, target, created.ID, command); err != nil {
		return result, errors.Join(err, workspace.close())
	}
	result.ExecutionID = created.ID
	if err := startDockerExec(ctx, target+"/start", command.Stdin, &result); err != nil {
		return result, errors.Join(err, workspace.close())
	}
	for {
		inspection, err := workspace.inspectExec(ctx, target, created.ID, command)
		if err != nil {
			return result, errors.Join(err, workspace.close())
		}
		if !*inspection.Running {
			if inspection.ExitCode == nil || *inspection.ExitCode < 0 || *inspection.ExitCode > 255 {
				return result, errors.Join(fmt.Errorf("Docker exec has no portable exit code"), workspace.close())
			}
			result.ExitCode = inspection.ExitCode
			break
		}
		select {
		case <-ctx.Done():
			return result, errors.Join(ctx.Err(), workspace.close())
		case <-time.After(25 * time.Millisecond):
		}
	}
	if err := workspace.requireRunning(ctx); err != nil {
		return result, errors.Join(err, workspace.close())
	}
	if *result.ExitCode != 0 {
		return result, &ExitError{Code: *result.ExitCode}
	}
	return result, nil
}

func (workspace *Workspace) inspectExec(ctx context.Context, target, id string, command Command) (execInspection, error) {
	var result execInspection
	if err := dockerAPI(ctx, http.MethodGet, target+"/json", nil, &result, nil); err != nil {
		return result, err
	}
	if result.ID != id || result.ContainerID != workspace.containerID || result.Running == nil || result.ProcessConfig == nil {
		return result, fmt.Errorf("Docker exec inspection lacks exact command authority: id=%q container=%q running_present=%t exit_present=%t process_present=%t", result.ID, result.ContainerID, result.Running != nil, result.ExitCode != nil, result.ProcessConfig != nil)
	}
	process := result.ProcessConfig
	if process.Entrypoint != command.Argv[0] || !slices.Equal(process.Arguments, command.Argv[1:]) ||
		process.User != "65532:65532" || process.Tty == nil || *process.Tty || process.Privileged == nil || *process.Privileged {
		return result, fmt.Errorf("Docker exec inspection differs from its declared command")
	}
	return result, nil
}

func (workspace *Workspace) commandEnvironment(overrides []string) ([]string, error) {
	values := make(map[string]string)
	for _, entry := range workspace.environment {
		name, _, found := strings.Cut(entry, "=")
		if _, duplicate := values[name]; duplicate || !found || !environmentNamePattern.MatchString(name) || !validText(entry) {
			return nil, fmt.Errorf("Docker image has an invalid or repeated environment name")
		}
		values[name] = entry
	}
	for _, entry := range overrides {
		name, _, _ := strings.Cut(entry, "=")
		values[name] = entry
	}
	result := make([]string, 0, len(values))
	for _, entry := range values {
		result = append(result, entry)
	}
	sort.Strings(result)
	return result, nil
}
