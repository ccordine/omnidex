package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationContextQuestionInventory WorkKind = "application_context_question_inventory"

	ApplicationNoRepositoryFactQuestionCandidates = "NO_REPOSITORY_FACT_QUESTION_CANDIDATES"
	ApplicationContextQuestionInventorySchemaV1   = "omnidex.application-context-question-inventory.v1"
)

type ApplicationContextQuestionInventoryInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
}

// ApplicationContextQuestionInventory is one bounded, untrusted semantic
// decomposition. Code owns candidate ordering, deduplication, authorization,
// repository evidence resolution, and queue exhaustion.
type ApplicationContextQuestionInventory struct {
	Schema          string   `json:"schema"`
	AuthoritySHA256 string   `json:"authority_sha256"`
	RawSHA256       string   `json:"raw_sha256"`
	Candidates      []string `json:"candidates"`
}

func NewApplicationContextQuestionInventoryJob(
	input ApplicationContextQuestionInventoryInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkApplicationContextQuestionInventory,
		input,
	)
}

func (input ApplicationContextQuestionInventoryInput) validate() error {
	if err := validateApplicationRequest(
		"application context question inventory", input.UserRequest,
	); err != nil {
		return err
	}
	if err := ValidatePathFreeModelContext(
		"application context question inventory request", input.UserRequest,
	); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if input.Context.RequestSHA256 != ExactObjectiveContextSHA(input.UserRequest) {
		return fmt.Errorf("application context question inventory request does not match context authority")
	}
	return nil
}

func BuildApplicationContextQuestionInventoryPrompt(
	input ApplicationContextQuestionInventoryInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := renderApplicationContextModelProjection(
		input.UserRequest,
		input.Context,
	)
	return strings.Join([]string{
		"Return one bounded source-ordered inventory of candidate repository-fact questions that may be necessary to interpret the immutable software request faithfully under the established context.",
		fmt.Sprintf("Return at most %d candidates, one complete raw interrogative sentence per non-empty line. Each line must ask for exactly one repository fact and be at most %d bytes.", MaxApplicationContextQuestionCandidates, maxApplicationEvidenceQuestionBytes),
		"Include only questions whose answers could supply a repository fact needed to interpret the request; omit implementation choices, speculative improvements, and quality opinions.",
		"When there are no candidate repository-fact questions, return only NO_REPOSITORY_FACT_QUESTION_CANDIDATES. Otherwise return question text only, with no JSON, labels, numbering, bullets, Markdown wrapping, explanation, or surrounding envelope.",
		"APPLICATION CONTEXT QUESTION INVENTORY AUTHORITY:\n" + projection,
	}, "\n\n"), nil
}

func DecodeApplicationContextQuestionInventory(
	input ApplicationContextQuestionInventoryInput,
	raw string,
) (ApplicationContextQuestionInventory, error) {
	var zero ApplicationContextQuestionInventory
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application context question inventory",
		raw,
		applicationContextQuestionInventoryMaximum(),
		true,
	)
	if err != nil {
		return zero, err
	}
	candidates := []string{}
	if leaf != ApplicationNoRepositoryFactQuestionCandidates {
		if strings.ContainsRune(leaf, '\r') {
			return zero, fmt.Errorf("application context question inventory must use LF line boundaries")
		}
		candidates = strings.Split(leaf, "\n")
		if len(candidates) > MaxApplicationContextQuestionCandidates {
			return zero, fmt.Errorf(
				"application context question inventory must contain 0..%d candidates",
				MaxApplicationContextQuestionCandidates,
			)
		}
		for index, candidate := range candidates {
			if err := validateApplicationContextQuestion(index, candidate); err != nil {
				return zero, err
			}
		}
	}
	authoritySHA256, err := applicationContextQuestionInventoryAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationContextQuestionInventory{
		Schema:          ApplicationContextQuestionInventorySchemaV1,
		AuthoritySHA256: authoritySHA256,
		RawSHA256:       ExactObjectiveContextSHA(leaf),
		Candidates:      append([]string{}, candidates...),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (inventory ApplicationContextQuestionInventory) ValidateFor(
	input ApplicationContextQuestionInventoryInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if inventory.Schema != ApplicationContextQuestionInventorySchemaV1 {
		return fmt.Errorf(
			"application context question inventory schema must be %q",
			ApplicationContextQuestionInventorySchemaV1,
		)
	}
	authoritySHA256, err := applicationContextQuestionInventoryAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if inventory.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application context question inventory authority hash does not match")
	}
	if inventory.Candidates == nil || len(inventory.Candidates) > MaxApplicationContextQuestionCandidates {
		return fmt.Errorf(
			"application context question inventory must contain 0..%d candidates",
			MaxApplicationContextQuestionCandidates,
		)
	}
	for index, candidate := range inventory.Candidates {
		if err := validateApplicationContextQuestion(index, candidate); err != nil {
			return err
		}
	}
	raw := ApplicationNoRepositoryFactQuestionCandidates
	if len(inventory.Candidates) > 0 {
		raw = strings.Join(inventory.Candidates, "\n")
	}
	if inventory.RawSHA256 != ExactObjectiveContextSHA(raw) {
		return fmt.Errorf("application context question inventory raw hash does not match")
	}
	return nil
}

func validateApplicationContextQuestion(index int, question string) error {
	if question == "" || question != strings.TrimSpace(question) ||
		len(question) > maxApplicationEvidenceQuestionBytes || !strings.HasSuffix(question, "?") {
		return fmt.Errorf(
			"application context question %d is not one valid interrogative sentence",
			index,
		)
	}
	if err := ValidatePathFreeModelContext("application context question", question); err != nil {
		return fmt.Errorf("application context question %d: %w", index, err)
	}
	return nil
}

func applicationContextQuestionInventoryMaximum() int {
	return max(
		len(ApplicationNoRepositoryFactQuestionCandidates),
		MaxApplicationContextQuestionCandidates*maxApplicationEvidenceQuestionBytes+
			MaxApplicationContextQuestionCandidates-1,
	)
}

func applicationContextQuestionInventoryAuthoritySHA256(
	input ApplicationContextQuestionInventoryInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application context question inventory authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
