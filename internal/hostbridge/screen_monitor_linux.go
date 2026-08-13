//go:build linux

package hostbridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var xrandrMonitorLine = regexp.MustCompile(`^\s*\d+:\s+([+\*]*)([\w-]+)\s+(\d+)/\d+x(\d+)/\d+\+(\d+)\+(\d+)`)

func listScreenMonitorPage(request ScreenMonitorPageRequest) (ScreenMonitorPage, error) {
	backend, err := screenMonitorBackend()
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	page, found, err := collectScreenMonitorPage(request, backend, func(visit func(ScreenMonitor) bool) (bool, int, error) {
		return runLinuxMonitorScan(backend, visit)
	})
	if err != nil {
		return ScreenMonitorPage{}, err
	}
	if !found {
		return ScreenMonitorPage{}, fmt.Errorf("%s returned no monitors", backend)
	}
	return page, nil
}

func findScreenMonitor(monitorID string) (ScreenMonitor, string, error) {
	backend, err := screenMonitorBackend()
	if err != nil {
		return ScreenMonitor{}, "", err
	}
	want := strings.TrimSpace(monitorID)
	var first *ScreenMonitor
	var selected *ScreenMonitor
	_, count, err := runLinuxMonitorScan(backend, func(monitor ScreenMonitor) bool {
		if first == nil {
			copy := monitor
			first = &copy
		}
		if (want == "" && monitor.Primary) || (want != "" && (monitor.ID == want || monitor.Name == want)) {
			copy := monitor
			selected = &copy
			return false
		}
		return true
	})
	if err != nil {
		return ScreenMonitor{}, "", err
	}
	if selected != nil {
		return *selected, backend, nil
	}
	if want == "" && first != nil {
		return *first, backend, nil
	}
	if count == 0 {
		return ScreenMonitor{}, "", fmt.Errorf("no monitors available")
	}
	return ScreenMonitor{}, "", fmt.Errorf("monitor %q not found", want)
}

func screenMonitorBackend() (string, error) {
	_, hyprctlErr := exec.LookPath("hyprctl")
	_, grimErr := exec.LookPath("grim")
	if hyprctlErr == nil && grimErr == nil {
		return "hyprland-grim", nil
	}
	if _, err := exec.LookPath("xrandr"); err == nil {
		return "x11", nil
	}
	return "", fmt.Errorf("no screen monitor backend available; install hyprctl and grim for Hyprland or xrandr for X11")
}

func runLinuxMonitorScan(backend string, visit func(ScreenMonitor) bool) (bool, int, error) {
	command := "xrandr"
	args := []string{"--listmonitors"}
	parser := scanXRandRMonitors
	if backend == "hyprland-grim" {
		command = "hyprctl"
		args = []string{"monitors", "-j"}
		parser = scanHyprlandMonitors
	} else if backend != "x11" {
		return false, 0, fmt.Errorf("unsupported screen backend %q", backend)
	}
	cmd := exec.Command(command, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, 0, err
	}
	if err := cmd.Start(); err != nil {
		return false, 0, err
	}
	complete, count, scanErr := parser(stdout, visit)
	if scanErr != nil || !complete {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return complete, count, scanErr
	}
	if err := cmd.Wait(); err != nil {
		return false, count, fmt.Errorf("%s monitor discovery failed: %w", command, err)
	}
	return true, count, nil
}

func scanHyprlandMonitors(reader io.Reader, visit func(ScreenMonitor) bool) (bool, int, error) {
	decoder := json.NewDecoder(reader)
	token, err := decoder.Token()
	if err != nil {
		return false, 0, err
	}
	opening, ok := token.(json.Delim)
	if !ok || opening != '[' {
		return false, 0, fmt.Errorf("hyprctl monitor response must be a JSON array")
	}
	count := 0
	for decoder.More() {
		var item struct {
			Name    string `json:"name"`
			Width   int    `json:"width"`
			Height  int    `json:"height"`
			X       int    `json:"x"`
			Y       int    `json:"y"`
			Focused bool   `json:"focused"`
		}
		if err := decoder.Decode(&item); err != nil {
			return false, count, err
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		count++
		if !visit(ScreenMonitor{ID: name, Name: name, Width: item.Width, Height: item.Height, X: item.X, Y: item.Y, Primary: item.Focused}) {
			return false, count, nil
		}
	}
	if _, err := decoder.Token(); err != nil {
		return false, count, err
	}
	return true, count, nil
}

func scanXRandRMonitors(reader io.Reader, visit func(ScreenMonitor) bool) (bool, int, error) {
	scanner := bufio.NewScanner(reader)
	count := 0
	for scanner.Scan() {
		match := xrandrMonitorLine.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		values := make([]int, 4)
		for index := range values {
			value, err := strconv.Atoi(match[index+3])
			if err != nil {
				return false, count, fmt.Errorf("invalid xrandr monitor geometry: %w", err)
			}
			values[index] = value
		}
		name := strings.TrimSpace(match[2])
		count++
		monitor := ScreenMonitor{ID: name, Name: name, Width: values[0], Height: values[1], X: values[2], Y: values[3], Primary: strings.Contains(match[1], "*")}
		if !visit(monitor) {
			return false, count, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, count, err
	}
	return true, count, nil
}
