package sourcebodyresponse

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Candidate is the only source text code may consider from one ordinary model
// response. A Markdown fence is tolerated as presentation noise; it is not a
// response grammar and is never required from the model.
type Candidate struct {
	Source string
	Fenced bool
}

// ExtractCandidate normalizes one ordinary response and, when fences are
// present, requires exactly one complete non-empty fenced region. Text outside
// that region is discarded. Multiple or malformed regions are ambiguous and
// fail rather than giving prose mutation authority.
func ExtractCandidate(raw string, maximumBytes int) (Candidate, error) {
	if maximumBytes < 1 {
		return Candidate{}, fmt.Errorf("source response maximum must be positive")
	}
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return Candidate{}, fmt.Errorf("source response must be valid UTF-8 without NUL bytes")
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return Candidate{}, fmt.Errorf("source response is empty")
	}
	if len(normalized) > maximumBytes {
		return Candidate{}, fmt.Errorf("source response exceeds %d bytes", maximumBytes)
	}

	lines := strings.Split(normalized, "\n")
	var blocks []string
	inFence := false
	fenceByte := byte(0)
	fenceWidth := 0
	start := 0
	for index, line := range lines {
		if !inFence {
			marker, width, ok := openingFence(line)
			if !ok {
				continue
			}
			inFence = true
			fenceByte = marker
			fenceWidth = width
			start = index + 1
			continue
		}
		if closingFence(line, fenceByte, fenceWidth) {
			block := strings.TrimSpace(strings.Join(lines[start:index], "\n"))
			if block == "" {
				return Candidate{}, fmt.Errorf("fenced source response is empty")
			}
			blocks = append(blocks, block)
			inFence = false
			fenceByte = 0
			fenceWidth = 0
		}
	}
	if inFence {
		return Candidate{}, fmt.Errorf("source response contains an unterminated fence")
	}
	if len(blocks) == 0 {
		if strings.Contains(normalized, "```") || strings.Contains(normalized, "~~~") {
			return Candidate{}, fmt.Errorf("source response contains a malformed fence")
		}
		return Candidate{Source: normalized}, nil
	}
	if len(blocks) != 1 {
		return Candidate{}, fmt.Errorf(
			"source response contains %d fenced regions; exactly one is required for deterministic extraction",
			len(blocks),
		)
	}
	if len(blocks[0]) > maximumBytes {
		return Candidate{}, fmt.Errorf("fenced source response exceeds %d bytes", maximumBytes)
	}
	return Candidate{Source: blocks[0], Fenced: true}, nil
}

func openingFence(line string) (byte, int, bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, false
	}
	width := repeatedPrefixWidth(trimmed, trimmed[0])
	if width < 3 {
		return 0, 0, false
	}
	return trimmed[0], width, true
}

func closingFence(line string, marker byte, minimumWidth int) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < minimumWidth || trimmed[0] != marker {
		return false
	}
	return repeatedPrefixWidth(trimmed, marker) == len(trimmed)
}

func repeatedPrefixWidth(value string, marker byte) int {
	width := 0
	for width < len(value) && value[width] == marker {
		width++
	}
	return width
}
