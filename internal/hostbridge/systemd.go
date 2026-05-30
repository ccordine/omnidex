package hostbridge

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	HostBridgeServiceName = "omni-host-bridge.service"
	hostBridgeConfigDir   = ".config/omni"
	hostBridgeEnvFile     = "host-bridge.env"
)

// ServiceInstallOptions configures systemd user service installation.
type ServiceInstallOptions struct {
	OmniPath string
	Listen   string
	Token    string
}

// ServicePaths holds resolved host bridge service file locations.
type ServicePaths struct {
	Home     string
	EnvFile  string
	UnitFile string
}

// ResolveServicePaths returns default user-level systemd paths for the host bridge.
func ResolveServicePaths() (ServicePaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ServicePaths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	return ServicePaths{
		Home:     home,
		EnvFile:  filepath.Join(home, hostBridgeConfigDir, hostBridgeEnvFile),
		UnitFile: filepath.Join(home, ".config/systemd/user", HostBridgeServiceName),
	}, nil
}

// ResolveOmniPath returns an absolute path to the omni binary.
func ResolveOmniPath(explicit string) (string, error) {
	clean := strings.TrimSpace(explicit)
	if clean != "" {
		if !filepath.IsAbs(clean) {
			abs, err := filepath.Abs(clean)
			if err != nil {
				return "", err
			}
			clean = abs
		}
		if stat, err := os.Stat(clean); err != nil {
			return "", fmt.Errorf("omni binary not found at %s: %w", clean, err)
		} else if stat.IsDir() {
			return "", fmt.Errorf("omni binary path is a directory: %s", clean)
		}
		return filepath.Clean(clean), nil
	}

	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	return filepath.Clean(path), nil
}

// RenderServiceEnvFile returns the host bridge environment file contents.
func RenderServiceEnvFile(listen, token string) string {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		listen = "0.0.0.0:8091"
	}
	token = strings.TrimSpace(token)

	var b strings.Builder
	b.WriteString("# Managed by `omni host service install`. Edit and run `omni host service restart`.\n")
	b.WriteString("# Dockerized core should use HOST_AGENT_URL=http://host.docker.internal:8091\n")
	fmt.Fprintf(&b, "HOST_AGENT_LISTEN=%s\n", listen)
	if token != "" {
		fmt.Fprintf(&b, "HOST_AGENT_TOKEN=%s\n", token)
	} else {
		b.WriteString("# HOST_AGENT_TOKEN=\n")
	}
	b.WriteString("# Optional GUI overrides if native folder picker fails under systemd:\n")
	b.WriteString("# DISPLAY=:0\n")
	b.WriteString("# WAYLAND_DISPLAY=wayland-0\n")
	b.WriteString("# Cursor SDK on host (required when systemd PATH lacks node/npm):\n")
	b.WriteString("# OMNI_CURSOR_NODE_BIN=/home/you/.local/share/mise/installs/node/VERSION/bin/node\n")
	b.WriteString("# OMNI_CURSOR_NPM_BIN=/home/you/.local/share/mise/installs/node/VERSION/bin/npm\n")
	b.WriteString("# PATH=/home/you/.local/share/mise/shims:/usr/bin:/bin\n")
	return b.String()
}

// RenderSystemdUnit returns a user systemd unit file for the host bridge.
func RenderSystemdUnit(omniPath, envFile string) string {
	return fmt.Sprintf(`[Unit]
Description=Omni host bridge (native directory picker + browse)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=-%s
ExecStart=%s host serve
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, envFile, omniPath)
}

// InstallSystemdService writes the unit/env files and enables the user service.
func InstallSystemdService(opts ServiceInstallOptions) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd service install is only supported on Linux")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl is required but was not found on PATH")
	}

	omniPath, err := ResolveOmniPath(opts.OmniPath)
	if err != nil {
		return err
	}
	paths, err := ResolveServicePaths()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(paths.EnvFile), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.UnitFile), 0o755); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}

	envContents := RenderServiceEnvFile(opts.Listen, opts.Token)
	if err := os.WriteFile(paths.EnvFile, []byte(envContents), 0o600); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}

	unitContents := RenderSystemdUnit(omniPath, paths.EnvFile)
	if err := os.WriteFile(paths.UnitFile, []byte(unitContents), 0o644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}

	if err := runSystemctlUser("daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctlUser("enable", "--now", HostBridgeServiceName); err != nil {
		return err
	}
	return nil
}

// UninstallSystemdService disables and removes the user systemd unit.
func UninstallSystemdService() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd service uninstall is only supported on Linux")
	}
	paths, err := ResolveServicePaths()
	if err != nil {
		return err
	}

	_ = runSystemctlUser("disable", "--now", HostBridgeServiceName)
	_ = os.Remove(paths.UnitFile)
	if err := runSystemctlUser("daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctlUser("reset-failed"); err != nil {
		return err
	}
	return nil
}

// RunSystemdServiceAction runs a systemctl --user action against the host bridge unit.
func RunSystemdServiceAction(action string, follow bool) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd service actions are only supported on Linux")
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "start":
		return runSystemctlUser("start", HostBridgeServiceName)
	case "stop":
		return runSystemctlUser("stop", HostBridgeServiceName)
	case "restart":
		return runSystemctlUser("restart", HostBridgeServiceName)
	case "status":
		return runSystemctlUser("status", HostBridgeServiceName)
	case "enable":
		return runSystemctlUser("enable", HostBridgeServiceName)
	case "disable":
		return runSystemctlUser("disable", HostBridgeServiceName)
	case "logs", "log":
		args := []string{"logs", "--no-pager"}
		if follow {
			args = append(args, "-f")
		}
		args = append(args, HostBridgeServiceName)
		return runSystemctlUser(args...)
	default:
		return fmt.Errorf("unknown service action %q (use install|uninstall|start|stop|restart|status|enable|disable|logs)", action)
	}
}

// ServiceInstalled reports whether the host bridge unit file exists.
func ServiceInstalled() (bool, error) {
	paths, err := ResolveServicePaths()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(paths.UnitFile)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func runSystemctlUser(args ...string) error {
	invocation := append([]string{"--user"}, args...)
	cmd := exec.Command("systemctl", invocation...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl %s: %w", strings.Join(invocation, " "), err)
	}
	return nil
}
