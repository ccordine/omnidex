package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	RepositorySearchTermSchemaV1    = "omnidex.repository-search-term.v1"
	maxRepositorySearchConceptBytes = 4 * 1024
	maxRepositorySearchTermBytes    = 256
)

type RepositorySearchTermInput struct {
	UnresolvedConcept string `json:"unresolved_concept"`
}

type RepositorySearchTermDecision struct {
	Schema string `json:"schema"`
	Term   string `json:"term"`
}

func NewRepositorySearchTermJob(input RepositorySearchTermInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkRepositorySearchTerm, input, input.validate)
}

func (input RepositorySearchTermInput) validate() error {
	return validateRepositorySearchText(
		"unresolved concept", input.UnresolvedConcept, maxRepositorySearchConceptBytes,
	)
}

func (decision RepositorySearchTermDecision) ValidateFor(input RepositorySearchTermInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RepositorySearchTermSchemaV1 {
		return fmt.Errorf("repository search term schema must be %q", RepositorySearchTermSchemaV1)
	}
	return validateRepositorySearchText("search term", decision.Term, maxRepositorySearchTermBytes)
}

func BuildRepositorySearchTermPrompt(input RepositorySearchTermInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Supply exactly one concise repository search term for the unresolved concept.",
		"The term may be a symbol name or a short implementation phrase. Do not decide how searching is executed or what happens next.",
		"UNRESOLVED_CONCEPT:\n" + input.UnresolvedConcept,
	}, "\n\n"), nil
}

func RepositorySearchTermResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "term"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": RepositorySearchTermSchemaV1,
			},
			"term": map[string]any{
				"type": "string", "minLength": 1, "maxLength": maxRepositorySearchTermBytes,
			},
		},
	)
}

func validateRepositorySearchText(label, value string, maximum int) error {
	if len(value) == 0 {
		return fmt.Errorf("repository search %s is empty", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("repository search %s exceeds %d bytes", label, maximum)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("repository search %s is not valid UTF-8", label)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("repository search %s contains NUL", label)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("repository search %s must be trimmed", label)
	}
	return nil
}
