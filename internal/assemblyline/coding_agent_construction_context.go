package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkApplicationConstructionContext WorkKind = "application_construction_context"
	ConstructionContextSchemaV1                 = "omnidex.application-construction-context.v1"
	NoConstructionConstraints                   = "NO_CONSTRUCTION_CONSTRAINTS"
	maxConstructionContextBytes                 = 16 * 1024
)

type ConstructionContextInput struct {
	UserRequest string `json:"user_request"`
}

type ConstructionFact struct {
	Kind  string `json:"kind"`
	Quote string `json:"quote"`
}

type ConstructionContext struct {
	Schema string             `json:"schema"`
	Facts  []ConstructionFact `json:"facts"`
}

func NewConstructionContextJob(input ConstructionContextInput) (PortableJob, error) {
	if err := input.validate(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkApplicationConstructionContext, input)
}

func (input ConstructionContextInput) validate() error {
	if err := validateApplicationRequest("construction context", input.UserRequest); err != nil {
		return err
	}
	return ValidatePathFreeModelContext("construction context request", input.UserRequest)
}

func BuildConstructionContextPrompt(input ConstructionContextInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"What explicit construction, delivery, documentation, and verification constraints does this request state?",
		"Extract only the smallest exact literal source phrases establishing a requested programming language, output artifact or filename, packaging or dependency restriction, documentation deliverable, or verification obligation. Preserve exact bytes and artifact tokens. Exclude runtime behaviors, API behavior, the complete request, plans, implementation suggestions, new requirements, approval, and control instructions.",
		"Return one constraint per line: one of language, output_file, packaging, documentation, verification, then a TAB character, then its exact source phrase. Do not quote or rephrase the phrase. Use " + NoConstructionConstraints + " when none exist.",
		"Request:\n" + input.UserRequest,
	}, "\n\n"), nil
}

func DecodeConstructionContext(input ConstructionContextInput, raw string) (ConstructionContext, error) {
	if err := input.validate(); err != nil {
		return ConstructionContext{}, err
	}
	text, err := decodeOrdinarySemanticText("construction context", raw, maxConstructionContextBytes)
	if err != nil {
		return ConstructionContext{}, err
	}
	context := ConstructionContext{Schema: ConstructionContextSchemaV1, Facts: []ConstructionFact{}}
	if text != NoConstructionConstraints {
		for _, line := range strings.Split(text, "\n") {
			kind, quote, ok := strings.Cut(line, "\t")
			if !ok {
				return ConstructionContext{}, fmt.Errorf("construction context line requires one kind and literal quote")
			}
			context.Facts = append(context.Facts, ConstructionFact{Kind: kind, Quote: quote})
		}
	}
	if err := context.ValidateFor(input); err != nil {
		return ConstructionContext{}, err
	}
	return context, nil
}

func (context ConstructionContext) ValidateFor(input ConstructionContextInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if context.Schema != ConstructionContextSchemaV1 {
		return fmt.Errorf("construction context schema is not registered")
	}
	seen := map[string]bool{}
	for _, fact := range context.Facts {
		switch fact.Kind {
		case "language", "output_file", "packaging", "documentation", "verification":
		default:
			return fmt.Errorf("construction context kind %q is not registered", fact.Kind)
		}
		if fact.Quote == "" || fact.Quote != strings.TrimSpace(fact.Quote) || strings.ContainsAny(fact.Quote, "\t\r\n") ||
			fact.Quote == strings.TrimSpace(input.UserRequest) || !strings.Contains(input.UserRequest, fact.Quote) || seen[fact.Quote] {
			return fmt.Errorf("construction constraint must be a unique exact proper source phrase")
		}
		if err := ValidatePathFreeModelContext("construction constraint quote", fact.Quote); err != nil {
			return err
		}
		seen[fact.Quote] = true
	}
	return nil
}
