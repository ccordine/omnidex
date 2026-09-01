package worker

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingBrowserExpectationMatcher struct {
	name      string
	negated   bool
	arguments *treesitter.Node
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateRequiredOutcomes(
	resultRelation string,
) error {
	if validator.executedAsserts == 0 {
		return fmt.Errorf("browser acceptance requires one structurally executed expect assertion containing a permitted screen query")
	}
	if resultRelation == assemblyline.ApplicationRequirementExplicitResultRelation &&
		validator.finalFireEventEnd == 0 {
		return fmt.Errorf(
			"browser acceptance for an explicit derived-result relation requires at least one receipt-grounded public interaction; current result-relation authority does not prove a no-interaction result",
		)
	}
	if validator.finalFireEventEnd > 0 &&
		resultRelation == assemblyline.ApplicationRequirementExplicitResultRelation {
		if !validator.hasOutputAssertionAfterFinalFireEvent() {
			if validator.hasUnprovenTextAssertionAfterFinalFireEvent() {
				return validator.unprovenTextOutcomeError()
			}
			return fmt.Errorf(
				"browser acceptance for an explicit derived-result relation requires one qualifying exact named status-output assertion after the final fireEvent interaction",
			)
		}
	}
	// A no-derived-result receipt does not authorize code to invent a semantic
	// postcondition. Its code-owned verifier may prove the initial surface and
	// dispatch compatible interactions, but it cannot claim an unknown result.
	if resultRelation == assemblyline.ApplicationRequirementExplicitResultRelation &&
		len(validator.outputAssertionStarts) == 0 {
		if len(validator.unprovenTextStarts) > 0 {
			return validator.unprovenTextOutcomeError()
		}
		return fmt.Errorf(
			"browser acceptance for an explicit derived-result relation requires one qualifying exact named status-output assertion",
		)
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) recordFireEvent(
	call *treesitter.Node,
) {
	if call != nil && call.EndByte() > validator.finalFireEventEnd {
		validator.finalFireEventEnd = call.EndByte()
	}
}

func (validator *directCodingBrowserAcceptanceQueryValidator) recordOutcomeAssertion(
	expectCall *treesitter.Node,
) {
	if expectCall == nil {
		return
	}
	qualified, exactOutput, unprovenText := validator.expectCallHasOutcomeShape(expectCall)
	if unprovenText {
		validator.unprovenTextStarts = append(
			validator.unprovenTextStarts, expectCall.StartByte(),
		)
	}
	if !qualified {
		return
	}
	if exactOutput {
		validator.outputAssertionStarts = append(
			validator.outputAssertionStarts, expectCall.StartByte(),
		)
	}
	validator.outcomeAssertionStarts = append(
		validator.outcomeAssertionStarts, expectCall.StartByte(),
	)
}

func (validator *directCodingBrowserAcceptanceQueryValidator) hasOutputAssertionAfterFinalFireEvent() bool {
	for _, start := range validator.outputAssertionStarts {
		if start > validator.finalFireEventEnd {
			return true
		}
	}
	return false
}

func (validator *directCodingBrowserAcceptanceQueryValidator) hasUnprovenTextAssertionAfterFinalFireEvent() bool {
	for _, start := range validator.unprovenTextStarts {
		if start > validator.finalFireEventEnd {
			return true
		}
	}
	return false
}

func (validator *directCodingBrowserAcceptanceQueryValidator) unprovenTextOutcomeError() error {
	if !validator.surfaceHasOutput() {
		return fmt.Errorf(
			"browser acceptance text query cannot qualify as a derived outcome without a code-proven named status output; the public-surface receipt has none",
		)
	}
	return fmt.Errorf(
		"browser acceptance getByText/findByText query cannot bind a derived outcome to an exact code-proven named status output; select that output by its exact status accessible name",
	)
}

func (validator *directCodingBrowserAcceptanceQueryValidator) surfaceHasOutput() bool {
	return len(validator.surface.Outputs) > 0
}

func (validator *directCodingBrowserAcceptanceQueryValidator) expectCallHasOutcomeShape(
	expectCall *treesitter.Node,
) (bool, bool, bool) {
	arguments := expectCall.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return false, false, false
	}
	selection := directCodingBrowserUnwrapOutcomeSelection(arguments.NamedChild(0))
	if selection == nil {
		return false, false, false
	}
	methodName, grounded := validator.screenQuerySelections[selection.Id()]
	if !grounded {
		return false, false, false
	}
	matcher, valid := validator.directExpectationMatcher(expectCall)
	if !valid {
		return false, false, false
	}
	if _, output := validator.outputSelections[selection.Id()]; output {
		qualified := directCodingBrowserOutputOutcomeMatcher(matcher, validator)
		return qualified, qualified, false
	}
	if methodName == "getByText" || methodName == "findByText" {
		// A passing text query does not prove that a named status output supplied
		// the text. Keep the assertion legal, but never grant it derived-outcome
		// authority without the exact output selection.
		return false, false, directCodingBrowserPresenceMatcher(matcher)
	}
	if validator.surfaceHasOutput() {
		return false, false, false
	}
	method, exists := directCodingBrowserScreenQueryMethods[methodName]
	if !exists || !method.role {
		return false, false, false
	}
	control, grounded := validator.roleSelections[selection.Id()]
	return grounded && directCodingBrowserRoleOutcomeMatcher(
		matcher, control, validator,
	), false, false
}

func directCodingBrowserUnwrapOutcomeSelection(node *treesitter.Node) *treesitter.Node {
	current := node
	for current != nil && current.Kind() == "parenthesized_expression" {
		if current.NamedChildCount() != 1 {
			return nil
		}
		current = current.NamedChild(0)
	}
	return current
}

func (validator *directCodingBrowserAcceptanceQueryValidator) directExpectationMatcher(
	expectCall *treesitter.Node,
) (directCodingBrowserExpectationMatcher, bool) {
	member := expectCall.Parent()
	if !directCodingBrowserMemberOwns(member, expectCall) {
		return directCodingBrowserExpectationMatcher{}, false
	}
	negated := false
	property := member.ChildByFieldName("property")
	if property == nil {
		return directCodingBrowserExpectationMatcher{}, false
	}
	if validator.text(property) == "not" {
		negated = true
		notMember := member
		member = notMember.Parent()
		if !directCodingBrowserMemberOwns(member, notMember) {
			return directCodingBrowserExpectationMatcher{}, false
		}
		property = member.ChildByFieldName("property")
		if property == nil {
			return directCodingBrowserExpectationMatcher{}, false
		}
	}
	if directCodingBrowserNodeHasChildKind(member, "optional_chain") {
		return directCodingBrowserExpectationMatcher{}, false
	}
	call := member.Parent()
	if call == nil || call.Kind() != "call_expression" ||
		!directCodingBrowserSameNode(call.ChildByFieldName("function"), member) {
		return directCodingBrowserExpectationMatcher{}, false
	}
	arguments := call.ChildByFieldName("arguments")
	if arguments == nil {
		return directCodingBrowserExpectationMatcher{}, false
	}
	return directCodingBrowserExpectationMatcher{
		name: validator.text(property), negated: negated, arguments: arguments,
	}, true
}

func directCodingBrowserMemberOwns(member *treesitter.Node, object *treesitter.Node) bool {
	return member != nil && member.Kind() == "member_expression" &&
		directCodingBrowserSameNode(member.ChildByFieldName("object"), object)
}

func directCodingBrowserPresenceMatcher(
	matcher directCodingBrowserExpectationMatcher,
) bool {
	if matcher.arguments.NamedChildCount() != 0 {
		return false
	}
	if matcher.negated {
		return matcher.name == "toBeNull"
	}
	switch matcher.name {
	case "toBeInTheDocument", "toBeVisible", "toBeTruthy", "toBeDefined":
		return true
	default:
		return false
	}
}

func directCodingBrowserRoleOutcomeMatcher(
	matcher directCodingBrowserExpectationMatcher,
	control directCodingBrowserPublicControl,
	validator *directCodingBrowserAcceptanceQueryValidator,
) bool {
	switch matcher.name {
	case "toBeChecked":
		return matcher.arguments.NamedChildCount() == 0 &&
			(control.Role == "checkbox" || control.Role == "radio")
	case "toBeInvalid", "toBeValid":
		return matcher.arguments.NamedChildCount() == 0 &&
			control.ValueKind != "action"
	case "toHaveDisplayValue", "toHaveValue":
		return !matcher.negated &&
			(control.ValueKind == "text" || control.ValueKind == "number" ||
				control.ValueKind == "selection") &&
			directCodingBrowserStaticMatcherScalar(matcher.arguments, validator)
	default:
		return false
	}
}

func directCodingBrowserStaticMatcherScalar(
	arguments *treesitter.Node,
	validator *directCodingBrowserAcceptanceQueryValidator,
) bool {
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return false
	}
	argument := arguments.NamedChild(0)
	if argument == nil {
		return false
	}
	if argument.Kind() == "string" {
		_, err := validator.exactString(argument)
		return err == nil
	}
	if argument.Kind() != "number" {
		return false
	}
	raw := validator.text(argument)
	value, err := strconv.ParseFloat(raw, 64)
	return err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
}
