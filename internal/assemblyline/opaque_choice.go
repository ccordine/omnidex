package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxOpaqueModelChoices = 256

// OpaqueModelChoice keeps the application value code-owned. The model sees
// only the generated letter and the minimum semantic description needed to
// choose it.
type OpaqueModelChoice struct {
	Description string
	value       string
}

func NewOpaqueModelChoice(description, value string) (OpaqueModelChoice, error) {
	description = strings.TrimSpace(description)
	if description == "" || !utf8.ValidString(description) ||
		strings.ContainsRune(description, '\x00') || len(description) > 4096 {
		return OpaqueModelChoice{}, fmt.Errorf(
			"opaque model choice requires one bounded semantic description",
		)
	}
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return OpaqueModelChoice{}, fmt.Errorf("opaque model choice requires one code-owned value")
	}
	return OpaqueModelChoice{Description: description, value: value}, nil
}

func ResolveSoleOpaqueModelChoice(choices []OpaqueModelChoice) (string, bool, error) {
	if err := validateOpaqueModelChoices(choices); err != nil {
		return "", false, err
	}
	if len(choices) != 1 {
		return "", false, nil
	}
	return choices[0].value, true, nil
}

func RenderOpaqueModelChoiceQuestion(
	question string,
	context []string,
	choices []OpaqueModelChoice,
) (string, error) {
	if err := validateOpaqueModelChoices(choices); err != nil {
		return "", err
	}
	question = strings.TrimSpace(question)
	if question == "" || !utf8.ValidString(question) || strings.ContainsRune(question, '\x00') {
		return "", fmt.Errorf("opaque model choice requires one exact semantic question")
	}
	parts := make([]string, 0, len(context)+len(choices)+2)
	for _, value := range context {
		value = strings.TrimSpace(value)
		if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return "", fmt.Errorf("opaque model choice context is invalid")
		}
		parts = append(parts, value)
	}
	parts = append(parts, question)
	for index, choice := range choices {
		parts = append(parts, opaqueModelChoiceID(index)+". "+choice.Description)
	}
	ids := make([]string, len(choices))
	for index := range choices {
		ids[index] = opaqueModelChoiceID(index)
	}
	parts = append(parts, "Answer with "+strings.Join(ids, " or ")+".")
	result := strings.Join(parts, "\n\n")
	if len(result) > maxPortableResourceBytes {
		return "", fmt.Errorf("opaque model choice input exceeds %d bytes", maxPortableResourceBytes)
	}
	return result, nil
}

func DecodeOpaqueModelChoice(raw string, choices []OpaqueModelChoice) (string, error) {
	if err := validateOpaqueModelChoices(choices); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"opaque model choice", raw, len(opaqueModelChoiceID(len(choices)-1)), false,
	)
	if err != nil {
		return "", err
	}
	for index, choice := range choices {
		if leaf == opaqueModelChoiceID(index) {
			return choice.value, nil
		}
	}
	return "", fmt.Errorf("opaque model choice ID %q is unavailable", leaf)
}

func opaqueModelChoiceResponseMaximum(choices []OpaqueModelChoice) (int, error) {
	if err := validateOpaqueModelChoices(choices); err != nil {
		return 0, err
	}
	return len(opaqueModelChoiceID(len(choices) - 1)), nil
}

func opaqueModelChoiceBuilderResponseMaximum(
	build func() ([]OpaqueModelChoice, error),
) (int, error) {
	choices, err := build()
	if err != nil {
		return 0, err
	}
	return opaqueModelChoiceResponseMaximum(choices)
}

func validateOpaqueModelChoices(choices []OpaqueModelChoice) error {
	if len(choices) < 1 || len(choices) > maxOpaqueModelChoices {
		return fmt.Errorf("opaque model choices require 1..%d options", maxOpaqueModelChoices)
	}
	seenDescriptions := make(map[string]struct{}, len(choices))
	seenValues := make(map[string]struct{}, len(choices))
	for index, choice := range choices {
		validated, err := NewOpaqueModelChoice(choice.Description, choice.value)
		if err != nil || validated != choice {
			if err == nil {
				err = fmt.Errorf("choice is not normalized")
			}
			return fmt.Errorf("opaque model choice %d: %w", index, err)
		}
		if _, exists := seenDescriptions[choice.Description]; exists {
			return fmt.Errorf("opaque model choice description %q is duplicated", choice.Description)
		}
		if _, exists := seenValues[choice.value]; exists {
			return fmt.Errorf("opaque model choice value is duplicated")
		}
		seenDescriptions[choice.Description] = struct{}{}
		seenValues[choice.value] = struct{}{}
	}
	return nil
}

func opaqueModelChoiceID(index int) string {
	if index < 0 || index >= maxOpaqueModelChoices {
		return ""
	}
	identifier := make([]byte, 0, 2)
	for number := index + 1; number > 0; number = (number - 1) / 26 {
		identifier = append(identifier, byte('A'+(number-1)%26))
	}
	for left, right := 0, len(identifier)-1; left < right; left, right = left+1, right-1 {
		identifier[left], identifier[right] = identifier[right], identifier[left]
	}
	return string(identifier)
}
