package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxTypeScriptRepairRegionLines       = 9
	maxTypeScriptRepairRegionBytes       = 2 * 1024
	maxTypeScriptRepairLineBytes         = 1024
	maxTypeScriptRepairAddedLines        = 4
	maxTypeScriptRepairRegionRadiusLines = 4
)

type TypeScriptFragmentRepairRegion struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Source    string `json:"source"`
}

type TypeScriptFragmentRepairDecision struct {
	ReplacementLines []string `json:"replacement_lines"`
}

func TypeScriptFragmentRepairResponseSchema(
	region TypeScriptFragmentRepairRegion,
) (map[string]any, error) {
	if err := region.validate(); err != nil {
		return nil, fmt.Errorf("TypeScript fragment repair response schema: %w", err)
	}
	lineCount := region.EndLine - region.StartLine + 1
	return objectSchema(
		[]string{"replacement_lines"},
		map[string]any{
			"replacement_lines": map[string]any{
				"type": "array", "minItems": 1,
				"maxItems": lineCount + maxTypeScriptRepairAddedLines,
				"items": map[string]any{
					"type": "string", "minLength": 1, "maxLength": maxTypeScriptRepairLineBytes,
				},
			},
		},
	), nil
}

func DecodeTypeScriptFragmentRepairDecision(
	region TypeScriptFragmentRepairRegion,
	raw string,
) (string, error) {
	if err := region.validate(); err != nil {
		return "", fmt.Errorf("TypeScript fragment repair response: %w", err)
	}
	if len(raw) > maxTypeScriptRepairRegionBytes*2 {
		return "", fmt.Errorf("TypeScript fragment repair response exceeds %d bytes", maxTypeScriptRepairRegionBytes*2)
	}
	var decision TypeScriptFragmentRepairDecision
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return "", fmt.Errorf("decode TypeScript fragment repair response: %w", err)
	}
	regionLines := region.EndLine - region.StartLine + 1
	if len(decision.ReplacementLines) < 1 {
		return "", fmt.Errorf("TypeScript fragment repair response requires at least one replacement line")
	}
	if len(decision.ReplacementLines) > regionLines+maxTypeScriptRepairAddedLines {
		return "", fmt.Errorf("TypeScript fragment repair response exceeds its local line authority")
	}
	flattened := make([]string, 0, len(decision.ReplacementLines))
	for index, fragment := range decision.ReplacementLines {
		if fragment == "" {
			return "", fmt.Errorf("TypeScript fragment repair response item %d is empty", index+1)
		}
		if !utf8.ValidString(fragment) || strings.Contains(fragment, "\r") {
			return "", fmt.Errorf("TypeScript fragment repair response item %d must be normalized UTF-8 without carriage returns", index+1)
		}
		if len(fragment) > maxTypeScriptRepairRegionBytes {
			return "", fmt.Errorf("TypeScript fragment repair response item %d exceeds %d bytes", index+1, maxTypeScriptRepairRegionBytes)
		}
		flattened = append(flattened, strings.Split(fragment, "\n")...)
	}
	if len(flattened) > regionLines+maxTypeScriptRepairAddedLines {
		return "", fmt.Errorf("TypeScript fragment repair response exceeds its flattened local line authority")
	}
	for index, line := range flattened {
		if len(line) > maxTypeScriptRepairLineBytes {
			return "", fmt.Errorf("TypeScript fragment repair response line %d exceeds %d bytes", index+1, maxTypeScriptRepairLineBytes)
		}
	}
	replacement := strings.Join(flattened, "\n")
	if strings.TrimSpace(replacement) == "" {
		return "", fmt.Errorf("TypeScript fragment repair response is empty")
	}
	if len(replacement) > maxTypeScriptRepairRegionBytes {
		return "", fmt.Errorf("TypeScript fragment repair replacement exceeds %d bytes", maxTypeScriptRepairRegionBytes)
	}
	if replacement == region.Source {
		return "", fmt.Errorf("TypeScript fragment repair response made no change")
	}
	return replacement, nil
}

func (region TypeScriptFragmentRepairRegion) validate() error {
	if region.StartLine < 1 || region.EndLine < region.StartLine {
		return fmt.Errorf("TypeScript repair region line range is invalid")
	}
	lineCount := region.EndLine - region.StartLine + 1
	if lineCount > maxTypeScriptRepairRegionLines {
		return fmt.Errorf("TypeScript repair region exceeds %d lines", maxTypeScriptRepairRegionLines)
	}
	if !utf8.ValidString(region.Source) || strings.Contains(region.Source, "\r") {
		return fmt.Errorf("TypeScript repair region source must be valid normalized UTF-8")
	}
	if strings.TrimSpace(region.Source) == "" {
		return fmt.Errorf("TypeScript repair region source is required")
	}
	if len(region.Source) > maxTypeScriptRepairRegionBytes {
		return fmt.Errorf("TypeScript repair region exceeds %d bytes", maxTypeScriptRepairRegionBytes)
	}
	if strings.Count(region.Source, "\n")+1 != lineCount {
		return fmt.Errorf("TypeScript repair region source does not match its line range")
	}
	return nil
}

func NewTypeScriptFragmentRepairRegion(
	current string,
	failure TypeScriptSyntaxFailure,
	radius int,
) (TypeScriptFragmentRepairRegion, error) {
	if radius < 1 || radius > maxTypeScriptRepairRegionRadiusLines {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf(
			"TypeScript repair region radius must be between 1 and %d lines",
			maxTypeScriptRepairRegionRadiusLines,
		)
	}
	source := strings.TrimSpace(strings.ReplaceAll(current, "\r\n", "\n"))
	if source == "" || !utf8.ValidString(source) {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf("TypeScript repair requires one valid current declaration")
	}
	lines := strings.Split(source, "\n")
	if failure.Line < 1 || failure.Line > len(lines) || failure.Column < 1 {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf(
			"TypeScript syntax location line %d column %d is outside the current declaration",
			failure.Line, failure.Column,
		)
	}
	start := failure.Line - radius
	if start < 1 {
		start = 1
	}
	end := failure.Line + radius
	if end > len(lines) {
		end = len(lines)
	}
	region := TypeScriptFragmentRepairRegion{
		StartLine: start,
		EndLine:   end,
		Source:    strings.Join(lines[start-1:end], "\n"),
	}
	if err := region.validate(); err != nil {
		return TypeScriptFragmentRepairRegion{}, err
	}
	return region, nil
}

func ApplyTypeScriptFragmentRepairRegion(
	current string,
	region TypeScriptFragmentRepairRegion,
	replacement string,
) (string, error) {
	if err := region.validate(); err != nil {
		return "", err
	}
	source := strings.TrimSpace(strings.ReplaceAll(current, "\r\n", "\n"))
	lines := strings.Split(source, "\n")
	if region.EndLine > len(lines) {
		return "", fmt.Errorf("TypeScript repair region is outside the current declaration")
	}
	if got := strings.Join(lines[region.StartLine-1:region.EndLine], "\n"); got != region.Source {
		return "", fmt.Errorf("TypeScript repair region no longer matches the current declaration")
	}
	replacement = strings.Trim(strings.ReplaceAll(replacement, "\r\n", "\n"), "\r\n")
	if strings.TrimSpace(replacement) == "" || !utf8.ValidString(replacement) {
		return "", fmt.Errorf("TypeScript repair replacement is empty or invalid UTF-8")
	}
	if len(replacement) > maxTypeScriptRepairRegionBytes {
		return "", fmt.Errorf("TypeScript repair replacement exceeds %d bytes", maxTypeScriptRepairRegionBytes)
	}
	replacementLines := strings.Split(replacement, "\n")
	regionLines := region.EndLine - region.StartLine + 1
	if len(replacementLines) > regionLines+maxTypeScriptRepairAddedLines {
		return "", fmt.Errorf("TypeScript repair replacement expands beyond its local line authority")
	}
	result := make([]string, 0, len(lines)-regionLines+len(replacementLines))
	result = append(result, lines[:region.StartLine-1]...)
	result = append(result, replacementLines...)
	result = append(result, lines[region.EndLine:]...)
	return strings.Join(result, "\n"), nil
}
