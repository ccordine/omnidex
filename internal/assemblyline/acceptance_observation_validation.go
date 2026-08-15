package assemblyline

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var acceptanceSyntaxKindPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var acceptanceOperationNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

func validateAcceptanceObservationSite(site AcceptanceObservationSite) error {
	if site.StatementKind == "" || site.StatementKind == "comment" ||
		!acceptanceSyntaxKindPattern.MatchString(site.StatementKind) ||
		site.StatementID == "" || len(site.Structure) == 0 || len(site.Operations) == 0 ||
		site.Operators == nil || site.Literals == nil {
		return fmt.Errorf("acceptance observation site %s is incomplete", site.ID)
	}
	for _, kind := range site.Structure {
		if kind == "comment" || !acceptanceSyntaxKindPattern.MatchString(kind) ||
			strings.Contains(kind, "identifier") {
			return fmt.Errorf("acceptance observation site %s contains non-portable structure", site.ID)
		}
	}
	for _, operation := range site.Operations {
		if !validAcceptanceObservationOperation(operation) {
			return fmt.Errorf("acceptance observation site %s contains unregistered operation %q", site.ID, operation)
		}
	}
	for _, operator := range site.Operators {
		if operator == "" || len(operator) > 4 || strings.ContainsAny(operator, "\r\n\t ") {
			return fmt.Errorf("acceptance observation site %s contains invalid operator", site.ID)
		}
	}
	for _, literal := range site.Literals {
		if !validAcceptanceLiteralKind(literal.Kind) || !utf8.ValidString(literal.Value) ||
			utf8.RuneCountInString(literal.Value) > maxApplicationCriterionRunes {
			return fmt.Errorf("acceptance observation site %s contains invalid literal", site.ID)
		}
	}
	return nil
}

func validAcceptanceObservationOperation(operation string) bool {
	if operation == "untrusted_call" {
		return true
	}
	prefix, name, found := strings.Cut(operation, ":")
	if !found {
		return false
	}
	if !acceptanceOperationNamePattern.MatchString(name) {
		return false
	}
	switch prefix {
	case "harness_call":
		if name == "expect" {
			return true
		}
		return acceptanceHarnessCall(name)
	case "testing_library_query":
		return isTestingLibraryQuery(name)
	case "fire_event", "expect_matcher":
		return true
	case "matcher_modifier":
		return acceptanceMatcherModifier(name)
	case "public_observation":
		return acceptancePublicObservationProperty(name)
	case "subscript_observation":
		return name == "index"
	case "query_option":
		return acceptanceSemanticFieldOperation("testing_library_query:getByText", name) != ""
	case "event_payload":
		return acceptanceSemanticFieldOperation("fire_event:change", name) != ""
	default:
		return false
	}
}

func acceptancePlatformOperation(operation string) bool {
	switch operation {
	case "harness_call:waitFor":
		return true
	default:
		return false
	}
}

func validAcceptanceLiteralKind(kind string) bool {
	switch kind {
	case "string", "number", "boolean", "null", "regular_expression", "template_string",
		"interpolated_template":
		return true
	default:
		return false
	}
}
