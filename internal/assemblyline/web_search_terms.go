package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const WebSearchTermsSchemaV1 = "omnidex.web-search-terms.v1"

type WebSearchTermsInput struct {
	ExactQuestion    string           `json:"exact_question"`
	Context          ObjectiveContext `json:"objective_context"`
	AttemptedQueries []string         `json:"attempted_queries"`
	MaxTerms         int              `json:"max_terms"`
	MaxTermBytes     int              `json:"max_term_bytes"`
}

type WebSearchTermsDecision struct {
	Schema string   `json:"schema"`
	Terms  []string `json:"terms"`
}

func NewWebSearchTermsJob(input WebSearchTermsInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkWebSearchTerms, input, input.validate)
}

func (input WebSearchTermsInput) validate() error {
	if err := validateExactWebQuestion(input.ExactQuestion); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if len(input.AttemptedQueries) > maxWebAttemptedQueries {
		return fmt.Errorf("web search terms allow at most %d attempted queries", maxWebAttemptedQueries)
	}
	seen := make(map[string]struct{}, len(input.AttemptedQueries))
	for index, query := range input.AttemptedQueries {
		if err := validateWebLine("attempted query", query, maxWebQueryBytes); err != nil {
			return fmt.Errorf("attempted query %d: %w", index, err)
		}
		identity := strings.ToLower(query)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("attempted query %q is duplicated", query)
		}
		seen[identity] = struct{}{}
	}
	if input.MaxTerms < 1 || input.MaxTerms > maxWebSearchTerms {
		return fmt.Errorf("web search term count bound must be 1..%d", maxWebSearchTerms)
	}
	if input.MaxTermBytes < 1 || input.MaxTermBytes > maxWebSearchTermBytes {
		return fmt.Errorf("web search term byte bound must be 1..%d", maxWebSearchTermBytes)
	}
	return nil
}

func (decision WebSearchTermsDecision) ValidateFor(input WebSearchTermsInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != WebSearchTermsSchemaV1 {
		return fmt.Errorf("web search terms schema must be %q", WebSearchTermsSchemaV1)
	}
	if len(decision.Terms) < 1 || len(decision.Terms) > input.MaxTerms {
		return fmt.Errorf("web search terms must contain 1..%d terms", input.MaxTerms)
	}
	seen := make(map[string]struct{}, len(input.AttemptedQueries)+len(decision.Terms))
	for _, query := range input.AttemptedQueries {
		seen[strings.ToLower(query)] = struct{}{}
	}
	for index, term := range decision.Terms {
		if err := validateWebLine("search term", term, input.MaxTermBytes); err != nil {
			return fmt.Errorf("search term %d: %w", index, err)
		}
		identity := strings.ToLower(term)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("web search term %q repeats an attempted or returned query", term)
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func DecodeWebSearchTermsDecision(input WebSearchTermsInput, raw string) (WebSearchTermsDecision, error) {
	decision, err := decodeWebStationDecision[WebSearchTermsDecision]("web search terms", raw)
	if err != nil {
		return WebSearchTermsDecision{}, err
	}
	if err := decision.ValidateFor(input); err != nil {
		return WebSearchTermsDecision{}, err
	}
	return decision, nil
}

func BuildWebSearchTermsPrompt(input WebSearchTermsInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode web search terms projection: %w", err)
	}
	return strings.Join([]string{
		"Resolve one named web search-term uncertainty.",
		"Return only bounded alternate query terms that do not repeat an attempted query. Do not choose providers, fetch anything, or decide subsequent work.",
		"WEB_SEARCH_TERM_GAP_JSON:\n" + string(projection),
	}, "\n\n"), nil
}

func WebSearchTermsResponseSchema(input WebSearchTermsInput) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	return objectSchema(
		[]string{"schema", "terms"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": WebSearchTermsSchemaV1},
			"terms": map[string]any{
				"type": "array", "minItems": 1, "maxItems": input.MaxTerms,
				"uniqueItems": true,
				"items": map[string]any{
					"type": "string", "minLength": 1, "maxLength": input.MaxTermBytes,
				},
			},
		},
	), nil
}
