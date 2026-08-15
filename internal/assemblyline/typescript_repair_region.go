package assemblyline

import (
	"errors"
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

// ErrTypeScriptRepairRegionUnrepresentable identifies a valid syntax location
// whose code-owned local window cannot fit the registered regional authority.
var ErrTypeScriptRepairRegionUnrepresentable = errors.New(
	"TypeScript repair region cannot fit its local authority",
)

type TypeScriptFragmentRepairRegion struct {
	Kind      TypeScriptFragmentRepairRegionKind `json:"kind"`
	StartLine int                                `json:"start_line"`
	EndLine   int                                `json:"end_line"`
	Source    string                             `json:"source"`
}

type TypeScriptFragmentRepairRegionKind string

const (
	TypeScriptRepairRegionSyntaxWindow  TypeScriptFragmentRepairRegionKind = "syntax_window"
	TypeScriptRepairRegionCompilerOwner TypeScriptFragmentRepairRegionKind = "compiler_owner"
)

func ProjectTypeScriptFragmentRepairResponse(
	region TypeScriptFragmentRepairRegion,
	raw string,
) (string, error) {
	if err := region.validate(); err != nil {
		return "", fmt.Errorf("TypeScript fragment repair response: %w", err)
	}
	projected, err := projectTypeScriptRepairRegionText(raw)
	if err != nil {
		return "", err
	}
	replacement := strings.Trim(strings.ReplaceAll(projected, "\r\n", "\n"), "\n")
	if !utf8.ValidString(replacement) || strings.Contains(replacement, "\r") {
		return "", fmt.Errorf("TypeScript fragment repair response must be normalized UTF-8 without carriage returns")
	}
	if strings.TrimSpace(replacement) == "" {
		return "", fmt.Errorf("TypeScript fragment repair response is empty")
	}
	if region.Kind == TypeScriptRepairRegionSyntaxWindow && len(replacement) > maxTypeScriptRepairRegionBytes {
		return "", fmt.Errorf("TypeScript fragment repair replacement exceeds %d bytes", maxTypeScriptRepairRegionBytes)
	}
	regionLines := region.EndLine - region.StartLine + 1
	lines := strings.Split(replacement, "\n")
	if region.Kind == TypeScriptRepairRegionSyntaxWindow && len(lines) > regionLines+maxTypeScriptRepairAddedLines {
		return "", fmt.Errorf("TypeScript fragment repair response exceeds its local line authority")
	}
	for index, line := range lines {
		if region.Kind != TypeScriptRepairRegionSyntaxWindow {
			break
		}
		if len(line) > maxTypeScriptRepairLineBytes {
			return "", fmt.Errorf("TypeScript fragment repair response line %d exceeds %d bytes", index+1, maxTypeScriptRepairLineBytes)
		}
	}
	if replacement == region.Source {
		return "", fmt.Errorf("TypeScript fragment repair response made no change")
	}
	return replacement, nil
}

func projectTypeScriptRepairRegionText(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) == "" || !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("TypeScript fragment repair response must be non-empty valid UTF-8 without NUL bytes")
	}
	segments := typeScriptResponseSegments(raw, true)
	if len(segments) == 1 && segments[0].fenced {
		return raw[segments[0].startByte:segments[0].endByte], nil
	}
	for _, segment := range segments {
		if segment.fenced {
			return "", fmt.Errorf("TypeScript fragment repair response mixes fenced source with other content")
		}
	}
	return raw, nil
}

func (region TypeScriptFragmentRepairRegion) validate() error {
	if region.Kind != TypeScriptRepairRegionSyntaxWindow && region.Kind != TypeScriptRepairRegionCompilerOwner {
		return fmt.Errorf("TypeScript repair region kind is invalid")
	}
	if region.StartLine < 1 || region.EndLine < region.StartLine {
		return fmt.Errorf("TypeScript repair region line range is invalid")
	}
	lineCount := region.EndLine - region.StartLine + 1
	if region.Kind == TypeScriptRepairRegionSyntaxWindow && lineCount > maxTypeScriptRepairRegionLines {
		return fmt.Errorf(
			"%w: exceeds %d lines",
			ErrTypeScriptRepairRegionUnrepresentable, maxTypeScriptRepairRegionLines,
		)
	}
	if !utf8.ValidString(region.Source) || strings.Contains(region.Source, "\r") {
		return fmt.Errorf("TypeScript repair region source must be valid normalized UTF-8")
	}
	if strings.TrimSpace(region.Source) == "" {
		return fmt.Errorf("TypeScript repair region source is required")
	}
	if region.Kind == TypeScriptRepairRegionSyntaxWindow && len(region.Source) > maxTypeScriptRepairRegionBytes {
		return fmt.Errorf(
			"%w: exceeds %d bytes",
			ErrTypeScriptRepairRegionUnrepresentable, maxTypeScriptRepairRegionBytes,
		)
	}
	if strings.Count(region.Source, "\n")+1 != lineCount {
		return fmt.Errorf("TypeScript repair region source does not match its line range")
	}
	for index, line := range strings.Split(region.Source, "\n") {
		if region.Kind != TypeScriptRepairRegionSyntaxWindow {
			break
		}
		if len(line) > maxTypeScriptRepairLineBytes {
			return fmt.Errorf(
				"%w: line %d exceeds %d bytes",
				ErrTypeScriptRepairRegionUnrepresentable, index+1, maxTypeScriptRepairLineBytes,
			)
		}
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
		Kind:      TypeScriptRepairRegionSyntaxWindow,
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
	if region.Kind == TypeScriptRepairRegionSyntaxWindow && len(replacement) > maxTypeScriptRepairRegionBytes {
		return "", fmt.Errorf("TypeScript repair replacement exceeds %d bytes", maxTypeScriptRepairRegionBytes)
	}
	replacementLines := strings.Split(replacement, "\n")
	regionLines := region.EndLine - region.StartLine + 1
	if region.Kind == TypeScriptRepairRegionSyntaxWindow && len(replacementLines) > regionLines+maxTypeScriptRepairAddedLines {
		return "", fmt.Errorf("TypeScript repair replacement expands beyond its local line authority")
	}
	result := make([]string, 0, len(lines)-regionLines+len(replacementLines))
	result = append(result, lines[:region.StartLine-1]...)
	result = append(result, replacementLines...)
	result = append(result, lines[region.EndLine:]...)
	return strings.Join(result, "\n"), nil
}
