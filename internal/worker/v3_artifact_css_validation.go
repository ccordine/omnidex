package worker

import "fmt"

func validateBalancedArtifact(
	label string,
	source []byte,
	blockComments bool,
	_ bool,
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
