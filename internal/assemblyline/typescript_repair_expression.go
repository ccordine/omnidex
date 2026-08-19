package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxTypeScriptRepairExpressionEvidence = 8
	maxTypeScriptRepairExpressionBytes    = 2 * 1024
)

// TypeScriptRepairExpressionEvidence is a compiler-checker projection for one
// expression at the exact diagnostic location. Code derives every field.
type TypeScriptRepairExpressionEvidence struct {
	Source             string   `json:"source"`
	InferredType       string   `json:"inferred_type"`
	ContextualType     string   `json:"contextual_type,omitempty"`
	IncompatibleTypes  []string `json:"incompatible_types,omitempty"`
	ReferencedBindings []string `json:"referenced_bindings,omitempty"`
}

func ValidateTypeScriptRepairExpressionEvidence(
	evidence []TypeScriptRepairExpressionEvidence,
) error {
	if len(evidence) > maxTypeScriptRepairExpressionEvidence {
		return fmt.Errorf(
			"TypeScript repair expression evidence exceeds %d entries",
			maxTypeScriptRepairExpressionEvidence,
		)
	}
	seen := make(map[string]struct{}, len(evidence))
	for index, item := range evidence {
		if err := validateTypeScriptRepairExpressionText("source", index, item.Source); err != nil {
			return err
		}
		if err := validateTypeScriptRepairExpressionText("inferred type", index, item.InferredType); err != nil {
			return err
		}
		identity := item.Source + "\x00" + item.InferredType + "\x00" + item.ContextualType
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("TypeScript repair expression evidence must be unique")
		}
		seen[identity] = struct{}{}
		if item.ContextualType != "" {
			if err := validateTypeScriptRepairExpressionText(
				"contextual type", index, item.ContextualType,
			); err != nil {
				return err
			}
		}
		prior := ""
		for _, incompatible := range item.IncompatibleTypes {
			if err := validateTypeScriptRepairExpressionText(
				"incompatible type", index, incompatible,
			); err != nil {
				return err
			}
			if prior != "" && incompatible <= prior {
				return fmt.Errorf(
					"TypeScript repair expression incompatible types must be uniquely sorted",
				)
			}
			prior = incompatible
		}
		if len(item.IncompatibleTypes) > 0 && item.ContextualType == "" {
			return fmt.Errorf(
				"TypeScript repair expression incompatible types require a contextual type",
			)
		}
		prior = ""
		for _, name := range item.ReferencedBindings {
			if err := validateTypeScriptRepairBindingText(
				"referenced binding", index, name,
			); err != nil {
				return err
			}
			if prior != "" && name <= prior {
				return fmt.Errorf(
					"TypeScript repair expression referenced bindings must be uniquely sorted",
				)
			}
			prior = name
		}
	}
	return nil
}

func validateTypeScriptRepairExpressionText(label string, index int, value string) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsAny(value, "\r\n") || strings.ContainsRune(value, 0) ||
		len(value) > maxTypeScriptRepairExpressionBytes {
		return fmt.Errorf(
			"TypeScript repair expression %d %s must be one bounded normalized line",
			index+1, label,
		)
	}
	return nil
}
