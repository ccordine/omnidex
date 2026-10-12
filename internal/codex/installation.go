package codex

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type Installation struct {
	Version      string `json:"version"`
	CanUpdate    bool   `json:"can_update"`
	UpdateMethod string `json:"update_method,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

func (client *Client) Installation(ctx context.Context) (Installation, error) {
	output, err := client.installationCommand(ctx, nil, "--version")
	if err != nil {
		return Installation{}, fmt.Errorf("read installed Codex version: %w", err)
	}
	version, err := codexVersion(output)
	if err != nil {
		return Installation{}, err
	}
	installation := Installation{Version: version}
	binding, err := bindCodexUpdate(client.command)
	if err != nil {
		installation.Reason = err.Error()
		return installation, nil
	}
	help, err := client.installationCommand(ctx, binding.environment(), "update", "--help")
	if ctx.Err() != nil {
		return Installation{}, ctx.Err()
	}
	if err != nil || !strings.Contains(help, "Usage: codex update") {
		installation.Reason = "This Codex installation does not advertise native updates."
		return installation, nil
	}
	installation.CanUpdate, installation.UpdateMethod = true, "native"
	return installation, nil
}

// Update delegates only to the installed release's advertised updater, with
// its existing package root and prefix bound. It never reinstalls credentials.
// The caller owns the idle-work guard and serializes update requests.
func (client *Client) Update(ctx context.Context) (Installation, error) {
	installation, err := client.Installation(ctx)
	if err != nil {
		return Installation{}, err
	}
	if !installation.CanUpdate {
		return installation, fmt.Errorf("Codex update unavailable: %s", installation.Reason)
	}
	binding, err := bindCodexUpdate(client.command)
	if err != nil {
		return installation, err
	}
	if _, err := client.installationCommand(ctx, binding.environment(), "update"); err != nil {
		return installation, fmt.Errorf("native Codex update failed: %w", err)
	}
	return client.Installation(ctx)
}

func codexVersion(output string) (string, error) {
	fields := strings.Fields(output)
	if len(fields) != 2 || fields[0] != "codex-cli" || fields[1] == "" {
		return "", fmt.Errorf("configured executable did not report a Codex CLI version")
	}
	return fields[1], nil
}

func (client *Client) installationCommand(ctx context.Context, environment []string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, client.command, arguments...)
	command.Dir, command.Stderr, command.Env = client.directory, io.Discard, environment
	command.WaitDelay = 5 * time.Second
	configureProcessLifetime(command)
	var output boundedInstallationOutput
	command.Stdout = &output
	if err := command.Run(); err != nil {
		return "", err
	}
	return string(output.bytes), nil
}

type boundedInstallationOutput struct{ bytes []byte }

func (output *boundedInstallationOutput) Write(data []byte) (int, error) {
	if len(data)+len(output.bytes) > 64<<10 {
		return 0, fmt.Errorf("Codex installation response exceeded its boundary")
	}
	output.bytes = append(output.bytes, data...)
	return len(data), nil
}
