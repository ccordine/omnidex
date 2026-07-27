package main

import (
	"errors"
	"strings"
)

func fetchWebText(url string) (string, error) {
	if commandExists("curl") {
		raw, err := runLocalCommand([]string{"curl", "-fsS", "--max-time", "8", url}, localShellCommandTimeout)
		if err == nil && strings.TrimSpace(raw) != "" {
			return raw, nil
		}
	}
	if commandExists("wget") {
		raw, err := runLocalCommand([]string{"wget", "-qO-", url}, localShellCommandTimeout)
		if err == nil && strings.TrimSpace(raw) != "" {
			return raw, nil
		}
	}
	return "", errors.New("no available fetch tool (`curl` or `wget`) could retrieve web content")
}

func trimLocalText(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if maxChars > 0 && len(value) > maxChars {
		return value[:maxChars]
	}
	return value
}

func showOpenPorts(detailed bool) (string, error) {
	command := []string{}
	useSudo := false

	switch {
	case commandExists("ss"):
		if detailed {
			command = []string{"ss", "-lntup"}
			useSudo = true
		} else {
			command = []string{"ss", "-lntu"}
		}
	case commandExists("netstat"):
		if detailed {
			command = []string{"netstat", "-lntup"}
			useSudo = true
		} else {
			command = []string{"netstat", "-lntu"}
		}
	case commandExists("lsof"):
		command = []string{"lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP"}
		if detailed {
			useSudo = true
		}
	default:
		return "", errors.New("no supported port-inspection tool found (need one of: ss, netstat, lsof)")
	}

	var (
		output string
		err    error
	)
	if useSudo {
		if err := ensureLocalPermission(permissionKeyShellSudo, "Allow running privileged network port inspection commands."); err != nil {
			return "", err
		}
		output, err = runLocalSudoCommand(command, localShellCommandTimeout)
	} else {
		output, err = runLocalCommand(command, localShellCommandTimeout)
	}
	if err != nil {
		return "", err
	}

	executed := strings.Join(command, " ")
	if useSudo {
		executed = "sudo " + executed
	}
	mode := "standard"
	if detailed {
		mode = "detailed"
	}
	lines := []string{
		"Open ports snapshot (" + mode + "):",
		"Executed: " + executed,
	}
	if strings.TrimSpace(output) != "" {
		lines = append(lines, "Output:")
		lines = append(lines, output)
	}
	return strings.Join(lines, "\n"), nil
}

func discoverLocalIPv4() []string {
	ips := []string{}
	seen := map[string]struct{}{}

	if commandExists("ip") {
		if raw, err := runLocalCommand([]string{"ip", "-4", "-o", "addr", "show", "scope", "global"}, localShellCommandTimeout); err == nil {
			for _, line := range strings.Split(raw, "\n") {
				fields := strings.Fields(line)
				for i := 0; i < len(fields)-1; i++ {
					if fields[i] != "inet" {
						continue
					}
					value := strings.TrimSpace(fields[i+1])
					if idx := strings.Index(value, "/"); idx > 0 {
						value = value[:idx]
					}
					if value == "" {
						continue
					}
					if _, ok := seen[value]; ok {
						continue
					}
					seen[value] = struct{}{}
					ips = append(ips, value)
				}
			}
		}
	}

	if len(ips) == 0 && commandExists("hostname") {
		if raw, err := runLocalCommand([]string{"hostname", "-I"}, localShellCommandTimeout); err == nil {
			for _, value := range strings.Fields(raw) {
				value = strings.TrimSpace(value)
				if value == "" || strings.Contains(value, ":") {
					continue
				}
				if _, ok := seen[value]; ok {
					continue
				}
				seen[value] = struct{}{}
				ips = append(ips, value)
			}
		}
	}

	return ips
}

func discoverPublicIPv4() string {
	if commandExists("curl") {
		if raw, err := runLocalCommand([]string{"curl", "-fsS", "--max-time", "5", "https://api.ipify.org"}, localShellCommandTimeout); err == nil {
			return strings.TrimSpace(raw)
		}
	}
	if commandExists("wget") {
		if raw, err := runLocalCommand([]string{"wget", "-qO-", "https://api.ipify.org"}, localShellCommandTimeout); err == nil {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}
