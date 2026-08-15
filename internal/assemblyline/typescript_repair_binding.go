package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TypeScriptRepairBinding is one compiler-proven value binding visible at the
// exact failing expression. Type and callable/member surfaces are projections
// from the same TypeScript checker program that produced the diagnostic.
type TypeScriptRepairBinding struct {
	Name               string   `json:"name"`
	Type               string   `json:"type"`
	CallableSignatures []string `json:"callable_signatures,omitempty"`
	Members            []string `json:"members,omitempty"`
}

func ValidateExactTypeScriptRepairBindings(bindings []TypeScriptRepairBinding) error {
	prior := ""
	for index, binding := range bindings {
		if err := validateTypeScriptRepairBindingText("name", index, binding.Name); err != nil {
			return err
		}
		if err := validateTypeScriptRepairBindingText("type", index, binding.Type); err != nil {
			return err
		}
		if index > 0 && binding.Name <= prior {
			return fmt.Errorf("TypeScript repair local bindings must be uniquely sorted by name")
		}
		prior = binding.Name
		if err := validateTypeScriptRepairBindingList("callable signature", index, binding.CallableSignatures); err != nil {
			return err
		}
		if err := validateTypeScriptRepairBindingList("member", index, binding.Members); err != nil {
			return err
		}
	}
	return nil
}

func validateTypeScriptRepairBindingList(label string, bindingIndex int, values []string) error {
	prior := ""
	for index, value := range values {
		if err := validateTypeScriptRepairBindingText(label, bindingIndex, value); err != nil {
			return err
		}
		if index > 0 && value <= prior {
			return fmt.Errorf("TypeScript repair binding %d %ss must be uniquely sorted", bindingIndex+1, label)
		}
		prior = value
	}
	return nil
}

func validateTypeScriptRepairBindingText(label string, bindingIndex int, value string) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsAny(value, "\r\n") || strings.ContainsRune(value, 0) {
		return fmt.Errorf("TypeScript repair binding %d %s must be one normalized line", bindingIndex+1, label)
	}
	return nil
}
