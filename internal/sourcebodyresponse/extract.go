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

// ExtractCandidates normalizes one ordinary response. When complete non-empty
// fences are present, each fenced region is returned as a candidate and text
// outside those regions is discarded. With no fences, the complete ordinary
// response is returned as one unfenced candidate.
func ExtractCandidates(raw string, maximumBytes int) ([]Candidate, error) {
	if maximumBytes < 1 {
		return nil, fmt.Errorf("source response maximum must be positive")
	}
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return nil, fmt.Errorf("source response must be valid UTF-8 without NUL bytes")
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return nil, fmt.Errorf("source response is empty")
	}
	if len(normalized) > maximumBytes {
		return nil, fmt.Errorf("source response exceeds %d bytes", maximumBytes)
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
				return nil, fmt.Errorf("fenced source response is empty")
			}
			blocks = append(blocks, block)
			inFence = false
			fenceByte = 0
			fenceWidth = 0
		}
	}
	if inFence {
		return nil, fmt.Errorf("source response contains an unterminated fence")
	}
	if len(blocks) == 0 {
		if strings.Contains(normalized, "```") || strings.Contains(normalized, "~~~") {
			return nil, fmt.Errorf("source response contains a malformed fence")
		}
		return []Candidate{{Source: normalized}}, nil
	}
	candidates := make([]Candidate, 0, len(blocks))
	for _, block := range blocks {
		if len(block) > maximumBytes {
			return nil, fmt.Errorf("fenced source response exceeds %d bytes", maximumBytes)
		}
		candidates = append(candidates, Candidate{Source: block, Fenced: true})
	}
	return candidates, nil
}

// ExtractCandidate requires exactly one candidate. Multiple fenced regions
// remain ambiguous unless a language-specific deterministic parser can prove
// that exactly one region has the required syntactic kind.
func ExtractCandidate(raw string, maximumBytes int) (Candidate, error) {
	candidates, err := ExtractCandidates(raw, maximumBytes)
	if err != nil {
		return Candidate{}, err
	}
	if len(candidates) != 1 {
		return Candidate{}, fmt.Errorf(
			"source response contains %d fenced regions; exactly one is required for deterministic extraction",
			len(candidates),
		)
	}
	return candidates[0], nil
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
