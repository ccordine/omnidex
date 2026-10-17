package codingagent

import (
	"encoding/json"
	"fmt"
	"strings"
)

type parserSourceUnit struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type parserUnitDisposition struct {
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	GroupStart string `json:"group_start"`
}

type parserScanResult struct {
	Units map[string]parserUnitDisposition `json:"units"`
}

// Unit identities and text are mechanical. Models classify identities and group
// wrapped continuation lines; they never reproduce bytes or invent offsets.
func parserSourceUnits(source string) []parserSourceUnit {
	lines := strings.SplitAfter(source, "\n")
	units := []parserSourceUnit{}
	fence := ""
	for _, line := range lines {
		if line == "" {
			continue
		}
		fragments := []string{line}
		trimmed := strings.TrimSpace(line)
		switch {
		case fence != "":
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
		case strings.HasPrefix(trimmed, "```"):
			fence = "```"
		case strings.HasPrefix(trimmed, "~~~"):
			fence = "~~~"
		default:
			fragments = parserSentenceFragments(line)
		}
		for _, fragment := range fragments {
			unit := parserSourceUnit{Text: fragment}
			if strings.TrimSpace(fragment) != "" {
				unit.ID = fmt.Sprintf("unit_%d", len(units)+1)
			}
			units = append(units, unit)
		}
	}
	return units
}

func parserScanParts(role Role, source string, result parserScanResult) ([]ParserPart, error) {
	units := parserSourceUnits(source)
	parts := []ParserPart{}
	seen := map[string]bool{}
	group := ""
	for _, unit := range units {
		if unit.ID == "" {
			parts = append(parts, ParserPart{Text: unit.Text, Kind: "ignore", Reason: "whitespace"})
			group = ""
			continue
		}
		disposition, exists := result.Units[unit.ID]
		if !exists || !contains(parserPartKinds(role), disposition.Kind) || strings.TrimSpace(disposition.Reason) == "" {
			return nil, fmt.Errorf("source unit %s has no valid explained disposition", unit.ID)
		}
		if disposition.Kind == "unknown" {
			// Unknown is non-authoritative parser commentary. Preserve the source
			// bytes, but do not let the commentary block the response.
			disposition.Kind = "ignore"
		}
		seen[unit.ID] = true
		if disposition.GroupStart == unit.ID {
			parts = append(parts, ParserPart{Text: unit.Text, Kind: disposition.Kind, Reason: disposition.Reason})
			group = unit.ID
		} else {
			if group == "" || disposition.GroupStart != group || len(parts) == 0 || parts[len(parts)-1].Kind != disposition.Kind {
				return nil, fmt.Errorf("source unit %s has an invalid or noncontiguous semantic group", unit.ID)
			}
			parts[len(parts)-1].Text += unit.Text
		}
	}
	if len(seen) != len(result.Units) {
		return nil, fmt.Errorf("parser invented or duplicated source unit identities")
	}
	if err := validateParserParts(role, source, parts); err != nil {
		return nil, err
	}
	return parts, nil
}

func decodeParserScan(call Assignment, content json.RawMessage) ([]ParserPart, error) {
	var scan parserScanResult
	if err := decodeContract(content, &scan); err != nil {
		return nil, err
	}
	return parserScanParts(call.Role, call.ParserStep.Source, scan)
}
