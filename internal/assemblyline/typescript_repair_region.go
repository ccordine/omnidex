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

// ErrTypeScriptFragmentRepairNoChange identifies a structurally valid
// replacement that is byte-identical to the exact mutable region.
var ErrTypeScriptFragmentRepairNoChange = errors.New(
	"TypeScript fragment repair response made no change",
)

type TypeScriptFragmentRepairRegion struct {
	Kind                TypeScriptFragmentRepairRegionKind   `json:"kind"`
	StartLine           int                                  `json:"start_line"`
	EndLine             int                                  `json:"end_line"`
	Source              string                               `json:"source"`
	Bindings            []TypeScriptRepairBinding            `json:"bindings,omitempty"`
	UnavailableBindings []TypeScriptRepairBinding            `json:"unavailable_bindings,omitempty"`
	ExpressionEvidence  []TypeScriptRepairExpressionEvidence `json:"expression_evidence,omitempty"`
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
	if err := validateExactTypeScriptRepairReplacement(raw); err != nil {
		return "", err
	}
	replacement := raw
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
	if err := validateTypeScriptCompilerRepairReplacementShape(region, replacement); err != nil {
		return "", err
	}
	if replacement == region.Source {
		return "", ErrTypeScriptFragmentRepairNoChange
	}
	return replacement, nil
}

func validateExactTypeScriptRepairReplacement(raw string) error {
	if raw == "" || !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) {
		return fmt.Errorf("TypeScript fragment repair response must be non-empty valid UTF-8 without NUL bytes")
	}
	if strings.ContainsRune(raw, '\r') {
		return fmt.Errorf("TypeScript fragment repair response must use exact LF source bytes")
	}
	if strings.TrimSpace(raw) == "" || strings.HasPrefix(raw, "\n") ||
		strings.HasSuffix(raw, "\n") || strings.TrimRight(raw, " \t") != raw {
		return fmt.Errorf("TypeScript fragment repair response must contain only the exact replacement source bytes")
	}
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			return fmt.Errorf("TypeScript fragment repair response must not contain a Markdown fence")
		}
	}
	return nil
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
	if region.Kind == TypeScriptRepairRegionCompilerOwner {
		if len(region.Bindings) == 0 {
			return fmt.Errorf("TypeScript compiler repair region requires exact local bindings")
		}
		if err := ValidateExactTypeScriptRepairBindings(region.Bindings); err != nil {
			return err
		}
		if err := ValidateExactTypeScriptRepairBindings(region.UnavailableBindings); err != nil {
			return fmt.Errorf("TypeScript compiler unavailable bindings: %w", err)
		}
		if err := ValidateTypeScriptRepairExpressionEvidence(region.ExpressionEvidence); err != nil {
			return err
		}
		available := make(map[string]struct{}, len(region.Bindings))
		for _, binding := range region.Bindings {
			available[binding.Name] = struct{}{}
		}
		for _, binding := range region.UnavailableBindings {
			if _, duplicate := available[binding.Name]; duplicate {
				return fmt.Errorf(
					"TypeScript compiler binding %q cannot be both available and unavailable",
					binding.Name,
				)
			}
		}
	} else if len(region.Bindings) != 0 || len(region.UnavailableBindings) != 0 ||
		len(region.ExpressionEvidence) != 0 {
		return fmt.Errorf("TypeScript syntax repair region cannot carry compiler evidence")
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
	if err := validateExactTypeScriptRepairReplacement(replacement); err != nil {
		return "", err
	}
	if region.Kind == TypeScriptRepairRegionSyntaxWindow && len(replacement) > maxTypeScriptRepairRegionBytes {
		return "", fmt.Errorf("TypeScript repair replacement exceeds %d bytes", maxTypeScriptRepairRegionBytes)
	}
	replacementLines := strings.Split(replacement, "\n")
	regionLines := region.EndLine - region.StartLine + 1
	if region.Kind == TypeScriptRepairRegionSyntaxWindow && len(replacementLines) > regionLines+maxTypeScriptRepairAddedLines {
		return "", fmt.Errorf("TypeScript repair replacement expands beyond its local line authority")
	}
	if err := validateTypeScriptCompilerRepairReplacementShape(region, replacement); err != nil {
		return "", err
	}
	result := make([]string, 0, len(lines)-regionLines+len(replacementLines))
	result = append(result, lines[:region.StartLine-1]...)
	result = append(result, replacementLines...)
	result = append(result, lines[region.EndLine:]...)
	return strings.Join(result, "\n"), nil
}
