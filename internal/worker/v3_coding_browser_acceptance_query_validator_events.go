package worker

import (
	"fmt"
	"math"
	"strconv"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

var directCodingBrowserFireEventMethods = map[string]struct{}{
	"click": {}, "change": {}, "input": {},
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateFireEventMember(
	member *treesitter.Node,
	methodName string,
) error {
	if _, allowed := directCodingBrowserFireEventMethods[methodName]; !allowed {
		return fmt.Errorf("browser acceptance fireEvent method %s is unsupported", methodName)
	}
	if directCodingBrowserNodeHasChildKind(member, "optional_chain") {
		return fmt.Errorf("browser acceptance fireEvent %s cannot use optional chaining", methodName)
	}
	call := member.Parent()
	if call == nil || call.Kind() != "call_expression" ||
		!directCodingBrowserSameNode(call.ChildByFieldName("function"), member) {
		return fmt.Errorf("browser acceptance fireEvent %s must be called directly", methodName)
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateFireEventCall(
	call *treesitter.Node,
	methodName string,
) error {
	if _, allowed := directCodingBrowserFireEventMethods[methodName]; !allowed {
		return fmt.Errorf("browser acceptance fireEvent method %s is unsupported", methodName)
	}
	if err := validator.requireExecuted(call, false); err != nil {
		return err
	}
	arguments := call.ChildByFieldName("arguments")
	wantArguments := uint(1)
	if methodName == "change" || methodName == "input" {
		wantArguments = 2
	}
	if arguments == nil || arguments.NamedChildCount() != wantArguments {
		return fmt.Errorf("browser acceptance fireEvent.%s requires exactly %d arguments", methodName, wantArguments)
	}
	control, err := validator.eventTargetControl(arguments.NamedChild(0))
	if err != nil {
		return fmt.Errorf("browser acceptance fireEvent.%s target: %w", methodName, err)
	}
	if methodName == "click" {
		if err := directCodingBrowserValidateClickControl(control); err != nil {
			return err
		}
		validator.recordFireEvent(call)
		return nil
	}
	payloadKind, payload, err := validator.staticTargetValue(arguments.NamedChild(1))
	if err != nil {
		return fmt.Errorf("browser acceptance fireEvent.%s payload: %w", methodName, err)
	}
	if err := directCodingBrowserValidateValueEvent(
		methodName, control, payloadKind, payload,
	); err != nil {
		return err
	}
	validator.recordFireEvent(call)
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) eventTargetControl(
	target *treesitter.Node,
) (directCodingBrowserPublicControl, error) {
	current := target
	for current != nil && current.Kind() == "parenthesized_expression" {
		if current.NamedChildCount() != 1 {
			return directCodingBrowserPublicControl{}, fmt.Errorf("target wrapper is ambiguous")
		}
		current = current.NamedChild(0)
	}
	if current != nil {
		if control, exists := validator.roleSelections[current.Id()]; exists {
			return control, nil
		}
	}
	return directCodingBrowserPublicControl{}, fmt.Errorf("target is not one exact grounded role query")
}

func directCodingBrowserValidateClickControl(control directCodingBrowserPublicControl) error {
	if control.ValueKind == "action" || control.ValueKind == "boolean" ||
		(control.ValueKind == "selection" && control.Role == "radio") {
		return nil
	}
	return fmt.Errorf(
		"browser acceptance click is incompatible with public control role %s value kind %s",
		control.Role, control.ValueKind,
	)
}

func directCodingBrowserValidateValueEvent(
	method string,
	control directCodingBrowserPublicControl,
	payloadKind string,
	payload string,
) error {
	switch control.ValueKind {
	case "text":
		if payloadKind != "string" {
			return fmt.Errorf("browser acceptance %s requires a static string for a text control", method)
		}
	case "number":
		if payloadKind != "string" && payloadKind != "number" {
			return fmt.Errorf("browser acceptance %s requires a static numeric value for a number control", method)
		}
		value, err := strconv.ParseFloat(payload, 64)
		if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
			return fmt.Errorf("browser acceptance %s value is not a finite static number", method)
		}
	case "selection":
		if method != "change" || payloadKind != "string" {
			return fmt.Errorf("browser acceptance selection controls require change with a static string")
		}
	default:
		return fmt.Errorf(
			"browser acceptance %s is incompatible with public control value kind %s",
			method, control.ValueKind,
		)
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) staticTargetValue(
	value *treesitter.Node,
) (string, string, error) {
	target, err := validator.exactObjectProperty(value, "target")
	if err != nil {
		return "", "", err
	}
	payload, err := validator.exactObjectProperty(target, "value")
	if err != nil {
		return "", "", err
	}
	switch payload.Kind() {
	case "string":
		value, err := validator.exactString(payload)
		if err != nil {
			return "", "", err
		}
		return "string", value, nil
	case "number":
		return "number", validator.text(payload), nil
	default:
		return "", "", fmt.Errorf("target.value must be one static string or number literal")
	}
}

func (validator *directCodingBrowserAcceptanceQueryValidator) exactObjectProperty(
	object *treesitter.Node,
	name string,
) (*treesitter.Node, error) {
	if object == nil || object.Kind() != "object" || object.NamedChildCount() != 1 ||
		object.NamedChild(0).Kind() != "pair" {
		return nil, fmt.Errorf("requires exact { %s: ... } object shape", name)
	}
	pair := object.NamedChild(0)
	key := pair.ChildByFieldName("key")
	value := pair.ChildByFieldName("value")
	if key == nil || value == nil || key.Kind() != "property_identifier" || validator.text(key) != name {
		return nil, fmt.Errorf("requires exact %s property", name)
	}
	return value, nil
}
