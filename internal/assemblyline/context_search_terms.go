package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const ContextSearchTermsSchemaV1 = "omnidex.context-search-terms.v1"

// ContextSearchTermsInput deliberately contains only the current instruction.
// Code owns every source queried with the returned strings.
type ContextSearchTermsInput struct {
	ExactInstruction string `json:"exact_instruction"`
}

type ContextSearchTermsDecision struct {
	Schema string   `json:"schema"`
	Terms  []string `json:"terms"`
}

func NewContextSearchTermsJob(input ContextSearchTermsInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkContextSearchTerms, input, input.validate)
}

func (input ContextSearchTermsInput) validate() error {
	return validateContextExactInstruction(input.ExactInstruction)
}

func (decision ContextSearchTermsDecision) ValidateFor(input ContextSearchTermsInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ContextSearchTermsSchemaV1 {
		return fmt.Errorf("context search terms schema must be %q", ContextSearchTermsSchemaV1)
	}
	if decision.Terms == nil {
		return fmt.Errorf("context search terms must be an explicit array")
	}
	if len(decision.Terms) > MaxContextSearchTerms {
		return fmt.Errorf("context search terms exceed %d items", MaxContextSearchTerms)
	}
	seen := make(map[string]struct{}, len(decision.Terms))
	for index, term := range decision.Terms {
		if err := validateContextSearchTerm(term); err != nil {
			return fmt.Errorf("context search term %d: %w", index, err)
		}
		identity := strings.ToLower(term)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("context search term %q is duplicated", term)
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func validateContextSearchTerm(term string) error {
	if err := validateContextText("search term", term, MaxContextSearchTermBytes); err != nil {
		return err
	}
	if strings.ContainsAny(term, "\r\n") {
		return fmt.Errorf("search term must be one line")
	}
	return nil
}

func DecodeContextSearchTermsDecision(
	input ContextSearchTermsInput,
	raw string,
) (ContextSearchTermsDecision, error) {
	decision, err := decodeContextSieveDecision[ContextSearchTermsDecision]("context search terms", raw)
	if err != nil {
		return ContextSearchTermsDecision{}, err
	}
	if err := decision.ValidateFor(input); err != nil {
		return ContextSearchTermsDecision{}, err
	}
	return decision, nil
}

func BuildContextSearchTermsPrompt(input ContextSearchTermsInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode context search terms projection: %w", err)
	}
	return strings.Join([]string{
		"Return zero to three concise query or concept strings that would help locate prior context required to interpret the exact current instruction.",
		"Return an empty terms array when the instruction is self-contained, including a greeting. For an anaphoric or elliptical instruction, name only the unresolved referent or prior action. Term order does not establish priority. Return no answer or explanation.",
		"CONTEXT_SEARCH_TERMS_JSON:\n" + string(projection),
	}, "\n\n"), nil
}

func ContextSearchTermsResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "terms"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": ContextSearchTermsSchemaV1},
			"terms": map[string]any{
				"type": "array", "minItems": 0, "maxItems": MaxContextSearchTerms,
				"uniqueItems": true,
				"items": map[string]any{
					"type": "string", "minLength": 1, "maxLength": MaxContextSearchTermBytes,
				},
			},
		},
	)
}
