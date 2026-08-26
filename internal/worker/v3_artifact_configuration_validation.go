package worker

import (
	"fmt"
	"strings"
	"unicode"
)

func validateBalancedArtifact(
	label string,
	source []byte,
	blockComments bool,
	hashComments bool,
) error {
	depth := 0
	quote := byte(0)
	escaped := false
	inBlockComment := false
	for index := 0; index < len(source); index++ {
		current := source[index]
		if inBlockComment {
			if current == '*' && index+1 < len(source) && source[index+1] == '/' {
				inBlockComment = false
				index++
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		if blockComments && current == '/' && index+1 < len(source) && source[index+1] == '*' {
			inBlockComment = true
			index++
			continue
		}
		if hashComments && current == '#' {
			for index < len(source) && source[index] != '\n' {
				index++
			}
			continue
		}
		switch current {
		case '\'', '"':
			quote = current
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return fmt.Errorf("%s contains an unmatched closing brace", label)
			}
		}
	}
	if inBlockComment {
		return fmt.Errorf("%s contains an unterminated block comment", label)
	}
	if quote != 0 {
		return fmt.Errorf("%s contains an unterminated quoted value", label)
	}
	if depth != 0 {
		return fmt.Errorf("%s contains %d unclosed block(s)", label, depth)
	}
	return nil
}

func validateNginxStatements(source []byte) error {
	depth := 0
	quote := byte(0)
	escaped := false
	pending := false
	for index := 0; index < len(source); index++ {
		current := source[index]
		if quote != 0 {
			pending = true
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '#' {
			for index < len(source) && source[index] != '\n' {
				index++
			}
			continue
		}
		switch current {
		case '\'', '"':
			quote = current
			pending = true
		case '{':
			if !pending {
				return fmt.Errorf("NGINX block has no directive")
			}
			pending = false
			depth++
		case ';':
			if !pending {
				return fmt.Errorf("NGINX contains an empty directive")
			}
			pending = false
		case '}':
			if pending {
				return fmt.Errorf("NGINX directive before closing brace lacks a semicolon")
			}
			depth--
		default:
			if !unicode.IsSpace(rune(current)) {
				pending = true
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("NGINX contains %d unclosed block(s)", depth)
	}
	if pending {
		return fmt.Errorf("NGINX final directive lacks a semicolon or block")
	}
	return nil
}

func validateDockerfileStatements(source []byte) error {
	logical := ""
	directives := 0
	heredocs := make([]string, 0)
	for index, raw := range strings.Split(string(source), "\n") {
		if len(heredocs) > 0 {
			if strings.TrimSpace(raw) == heredocs[0] {
				heredocs = heredocs[1:]
			}
			continue
		}
		line := strings.TrimSpace(raw)
		if logical == "" && (line == "" || strings.HasPrefix(line, "#")) {
			continue
		}
		continued := dockerfileLineContinues(line)
		if continued {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\\"))
		}
		if logical != "" {
			logical += " "
		}
		logical += line
		if continued {
			continue
		}
		fields := strings.Fields(logical)
		if len(fields) < 2 || !validDockerfileDirective(fields[0]) {
			return fmt.Errorf("invalid Dockerfile instruction ending at line %d", index+1)
		}
		directives++
		heredocs = dockerfileHeredocDelimiters(fields[1:])
		logical = ""
	}
	if logical != "" {
		return fmt.Errorf("Dockerfile ends inside a continued instruction")
	}
	if len(heredocs) > 0 {
		return fmt.Errorf("Dockerfile heredoc %q is unterminated", heredocs[0])
	}
	if directives == 0 {
		return fmt.Errorf("Dockerfile contains no instruction")
	}
	return nil
}

func dockerfileLineContinues(line string) bool {
	count := 0
	for index := len(line) - 1; index >= 0 && line[index] == '\\'; index-- {
		count++
	}
	return count%2 == 1
}

func validDockerfileDirective(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !unicode.IsLetter(char) {
			return false
		}
	}
	_, registered := dockerfileDirectives[strings.ToUpper(value)]
	return registered
}

var dockerfileDirectives = map[string]struct{}{
	"ADD": {}, "ARG": {}, "CMD": {}, "COPY": {}, "ENTRYPOINT": {},
	"ENV": {}, "EXPOSE": {}, "FROM": {}, "HEALTHCHECK": {}, "LABEL": {},
	"MAINTAINER": {}, "ONBUILD": {}, "RUN": {}, "SHELL": {},
	"STOPSIGNAL": {}, "USER": {}, "VOLUME": {}, "WORKDIR": {},
}

func dockerfileHeredocDelimiters(fields []string) []string {
	delimiters := make([]string, 0)
	for _, field := range fields {
		if !strings.HasPrefix(field, "<<") {
			continue
		}
		delimiter := strings.TrimPrefix(field, "<<")
		delimiter = strings.TrimPrefix(delimiter, "-")
		delimiter = strings.Trim(delimiter, "'\"")
		if delimiter != "" {
			delimiters = append(delimiters, delimiter)
		}
	}
	return delimiters
}
