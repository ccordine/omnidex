package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func showSystemSummary() string {
	cwd := ""
	if dir, err := os.Getwd(); err == nil {
		cwd = strings.TrimSpace(dir)
	}
	snapshot := discoverHostEnvironmentSnapshot(cwd)
	lines := []string{
		"System summary:",
		"user=" + safeValue(snapshot.User, "unknown"),
		"identity=" + safeValue(snapshot.Identity, "unknown"),
		"os=" + safeValue(snapshot.Distro, snapshot.OS),
		"kernel=" + safeValue(snapshot.Kernel, "unknown"),
		"arch=" + safeValue(snapshot.Arch, "unknown"),
		"shell=" + safeValue(snapshot.Shell, "unknown"),
		"cwd=" + safeValue(snapshot.CWD, "unknown"),
		"package_manager=" + safeValue(snapshot.PackageManager, "(none)"),
		"local_time=" + safeValue(snapshot.NowLocal, "unknown"),
		"timezone=" + safeValue(snapshot.Timezone, "unknown"),
	}
	return strings.Join(lines, "\n")
}

func showRunningProcesses() (string, error) {
	lines := []string{"Running processes snapshot:"}
	strategies := make([]string, 0, 3)
	sections := make([]string, 0, 3)
	notes := make([]string, 0, 3)

	if output, executed, err := collectTopProcessSnapshot(); err == nil {
		strategies = append(strategies, "top")
		sections = append(sections, fmt.Sprintf("Strategy: top\nExecuted: %s\nOutput:\n%s", executed, output))
	} else {
		notes = append(notes, "top strategy unavailable: "+sanitizeSudoReasonText(err.Error()))
	}

	if output, executed, err := collectPSProcessSnapshot(); err == nil {
		strategies = append(strategies, "ps")
		sections = append(sections, fmt.Sprintf("Strategy: ps\nExecuted: %s\nOutput:\n%s", executed, output))
	} else {
		notes = append(notes, "ps strategy unavailable: "+sanitizeSudoReasonText(err.Error()))
	}

	if output, executed, err := collectRunningServiceSnapshot(); err == nil {
		strategies = append(strategies, "services")
		sections = append(sections, fmt.Sprintf("Strategy: service inventory\nExecuted: %s\nOutput:\n%s", executed, output))
	}

	if len(strategies) == 0 {
		return "", errors.New("unable to inspect running processes with available local tools (tried top/ps)")
	}

	lines = append(lines, "strategies="+strings.Join(strategies, ", "))
	lines = append(lines, sections...)
	if len(notes) > 0 {
		lines = append(lines, "notes:")
		for _, note := range notes {
			lines = append(lines, "- "+note)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func collectTopProcessSnapshot() (string, string, error) {
	if !commandExists("top") {
		return "", "", errors.New("`top` is not available")
	}
	attempts := [][]string{
		{"top", "-b", "-n", "1", "-w", "120"},
		{"top", "-b", "-n", "1"},
		{"top", "-l", "1", "-n", "25"},
		{"top", "-l", "1"},
	}
	var lastErr error
	for _, args := range attempts {
		raw, err := runLocalCommandMax(args, 10*time.Second, 12000)
		if err != nil {
			lastErr = err
			continue
		}
		if strings.TrimSpace(raw) == "" {
			lastErr = errors.New("empty output")
			continue
		}
		return trimCommandOutputLines(raw, 45), strings.Join(args, " "), nil
	}
	if lastErr == nil {
		lastErr = errors.New("no usable output")
	}
	return "", "", lastErr
}

func collectPSProcessSnapshot() (string, string, error) {
	if !commandExists("ps") {
		return "", "", errors.New("`ps` is not available")
	}
	attempts := [][]string{
		{"ps", "-eo", "pid,ppid,user,comm,%cpu,%mem,etime,state", "--sort=-%cpu"},
		{"ps", "-Ao", "pid,ppid,user,comm,%cpu,%mem,etime,state", "-r"},
		{"ps", "aux"},
	}
	var lastErr error
	for _, args := range attempts {
		raw, err := runLocalCommandMax(args, 8*time.Second, 10000)
		if err != nil {
			lastErr = err
			continue
		}
		if strings.TrimSpace(raw) == "" {
			lastErr = errors.New("empty output")
			continue
		}
		return trimCommandOutputLines(raw, 35), strings.Join(args, " "), nil
	}
	if lastErr == nil {
		lastErr = errors.New("no usable output")
	}
	return "", "", lastErr
}

func collectRunningServiceSnapshot() (string, string, error) {
	if commandExists("systemctl") {
		args := []string{"systemctl", "--type=service", "--state=running", "--no-pager", "--no-legend"}
		raw, err := runLocalCommandMax(args, 8*time.Second, 8000)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(raw) == "" {
			return "", "", errors.New("empty output")
		}
		return trimCommandOutputLines(raw, 30), strings.Join(args, " "), nil
	}
	return "", "", errors.New("no supported service manager command found")
}

func trimCommandOutputLines(raw string, maxLines int) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		remaining := len(lines) - maxLines
		lines = append(lines[:maxLines], fmt.Sprintf("...(%d more lines truncated)", remaining))
	}
	return strings.Join(lines, "\n")
}

func showNetworkIP() (string, error) {
	local := discoverLocalIPv4()
	public := discoverPublicIPv4()

	lines := []string{"Network IP snapshot:"}
	if len(local) == 0 {
		lines = append(lines, "local_ipv4=(unavailable)")
	} else {
		lines = append(lines, "local_ipv4="+strings.Join(local, ","))
	}
	if strings.TrimSpace(public) == "" {
		lines = append(lines, "public_ipv4=(unavailable)")
	} else {
		lines = append(lines, "public_ipv4="+public)
	}
	return strings.Join(lines, "\n"), nil
}
