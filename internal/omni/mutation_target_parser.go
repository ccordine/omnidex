package omni

import (
	"fmt"
	"strings"
)

func mutationWriteTargetPaths(command string) ([]string, error) {
	targets, err := shellRedirectionWriteTargets(command)
	if err != nil {
		return nil, err
	}
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) == 0 || cleanCommandPathToken(segment[0]) != "touch" {
			continue
		}
		for _, arg := range segment[1:] {
			if !strings.HasPrefix(arg, "-") {
				targets = append(targets, arg)
			}
		}
	}
	return uniqueNonEmptyStrings(targets), nil
}

func shellRedirectionWriteTargets(command string) ([]string, error) {
	command, err := shellCommandWithoutHeredocBodies(command)
	if err != nil {
		return nil, err
	}
	targets := []string{}
	quote := byte(0)
	escaped := false
	for i := 0; i < len(command); i++ {
		current := command[i]
		if escaped {
			escaped = false
			continue
		}
		if current == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current != '>' {
			continue
		}
		for i+1 < len(command) && command[i+1] == '>' {
			i++
		}
		target, next, err := readShellRedirectionTarget(command, i+1)
		if err != nil {
			return nil, err
		}
		i = next - 1
		if strings.HasPrefix(target, "&") {
			continue
		}
		targets = append(targets, target)
	}
	if quote != 0 {
		return nil, fmt.Errorf("shell redirection contains an unterminated quote")
	}
	return targets, nil
}

func shellCommandWithoutHeredocBodies(command string) (string, error) {
	lines := strings.Split(command, "\n")
	masked := make([]string, len(lines))
	heredocDelimiter := ""
	for lineIndex, line := range lines {
		if heredocDelimiter != "" {
			if strings.TrimSpace(line) == heredocDelimiter {
				heredocDelimiter = ""
			}
			continue
		}
		masked[lineIndex] = line
		quote := byte(0)
		escaped := false
		for i := 0; i < len(line); i++ {
			current := line[i]
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' && quote != '\'' {
				escaped = true
				continue
			}
			if quote != 0 {
				if current == quote {
					quote = 0
				}
				continue
			}
			if current == '\'' || current == '"' {
				quote = current
				continue
			}
			if current == '<' && i+1 < len(line) && line[i+1] == '<' {
				delimiter, end := parseShellHeredocDelimiter(line, i+2)
				if delimiter != "" {
					heredocDelimiter = delimiter
					i = end - 1
				}
			}
		}
	}
	if heredocDelimiter != "" {
		return "", fmt.Errorf("shell redirection contains unterminated heredoc %q", heredocDelimiter)
	}
	return strings.Join(masked, "\n"), nil
}

func readShellRedirectionTarget(command string, start int) (string, int, error) {
	for start < len(command) && (command[start] == ' ' || command[start] == '\t') {
		start++
	}
	if start >= len(command) || strings.ContainsRune(";|<>", rune(command[start])) {
		return "", start, fmt.Errorf("shell redirection is missing target")
	}
	quote := byte(0)
	escaped := false
	target := strings.Builder{}
	for i := start; i < len(command); i++ {
		current := command[i]
		if escaped {
			target.WriteByte(current)
			escaped = false
			continue
		}
		if current == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if current == quote {
				quote = 0
			} else {
				target.WriteByte(current)
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current == ' ' || current == '\t' || strings.ContainsRune(";|<>", rune(current)) {
			if target.Len() == 0 {
				return "", i, fmt.Errorf("shell redirection is missing target")
			}
			return target.String(), i, nil
		}
		target.WriteByte(current)
	}
	if escaped || quote != 0 {
		return "", len(command), fmt.Errorf("shell redirection target contains an unterminated quote or escape")
	}
	if target.Len() == 0 {
		return "", len(command), fmt.Errorf("shell redirection is missing target")
	}
	return target.String(), len(command), nil
}
