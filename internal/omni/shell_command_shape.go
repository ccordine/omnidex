package omni

import "strings"

func hasTopLevelShellSemicolon(command string) bool {
	var quote byte
	escaped := false
	heredocDelimiter := ""
	for _, line := range strings.Split(command, "\n") {
		if heredocDelimiter != "" {
			if strings.TrimSpace(line) == heredocDelimiter {
				heredocDelimiter = ""
			}
			continue
		}
		for i := 0; i < len(line); i++ {
			current := line[i]
			if escaped {
				escaped = false
				continue
			}
			if quote != '\'' && current == '\\' {
				escaped = true
				continue
			}
			if quote != 0 {
				if current == quote {
					quote = 0
				}
				continue
			}
			switch current {
			case '\'', '"':
				quote = current
			case '#':
				if i == 0 || line[i-1] == ' ' || line[i-1] == '\t' {
					i = len(line)
				}
			case '<':
				if i+1 < len(line) && line[i+1] == '<' {
					if delimiter, end := parseShellHeredocDelimiter(line, i+2); delimiter != "" {
						heredocDelimiter = delimiter
						i = end - 1
					}
				}
			case ';':
				return true
			}
		}
	}
	return false
}

func parseShellHeredocDelimiter(line string, start int) (string, int) {
	i := start
	if i < len(line) && line[i] == '-' {
		i++
	}
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) {
		return "", start
	}
	if line[i] == '\'' || line[i] == '"' {
		quote := line[i]
		i++
		begin := i
		for i < len(line) && line[i] != quote {
			i++
		}
		if i >= len(line) {
			return "", start
		}
		return line[begin:i], i + 1
	}
	begin := i
	for i < len(line) && !strings.ContainsRune(" \t;|&<>", rune(line[i])) {
		i++
	}
	return line[begin:i], i
}
