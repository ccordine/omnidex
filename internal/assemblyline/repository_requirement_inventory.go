package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkRepositoryRequirementInventory WorkKind = "repository_requirement_inventory"

	MaxRepositoryRequirementLeaves              = maxRequirementCount
	MaxRepositoryRequirementInventoryCandidates = MaxRepositoryRequirementLeaves * 3
	maxRepositoryRequirementInventoryBytes      = MaxRepositoryRequirementInventoryCandidates*maxRequirementQuoteBytes +
		MaxRepositoryRequirementInventoryCandidates - 1

	RepositoryRequirementInventorySchemaV1 = "omnidex.repository-requirement-inventory.v1"
)

// RepositoryRequirementInventory is an untrusted, source-ordered inventory.
// Its candidates do not become requirements until the code-owned candidate
// queue applies the separate candidate relation and deterministic duplicate
// checks.
type RepositoryRequirementInventory struct {
	Schema          string   `json:"schema"`
	AuthoritySHA256 string   `json:"authority_sha256"`
	RawSHA256       string   `json:"raw_sha256"`
	Candidates      []string `json:"candidates"`
}

func NewRepositoryRequirementInventoryJob(
	input RepositoryRequirementInterpretationInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkRepositoryRequirementInventory,
		input,
	)
}

func BuildRepositoryRequirementInventoryPrompt(
	input RepositoryRequirementInterpretationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := renderImmutableUserRequestModelProjection(input.UserRequest)
	return strings.Join([]string{
		"Return one bounded source-ordered inventory of candidate semantically separable clauses explicitly present in the immutable existing-repository request.",
		"Include requested changes, preservation constraints, background statements, questions, advice, builder directions, verification directions, and repeated clauses. Preserve the request's clause boundaries without merging, deduplicating, paraphrasing, or adding a clause.",
		"Each line must be one exact contiguous quote from the immutable request. Split coordinated meanings at source boundaries when each quoted phrase remains meaningful with the immutable request. Preserve source order and exact bytes.",
		"Include only clauses present in the request; omit speculative additions.",
		fmt.Sprintf("Return between 1 and %d non-empty raw candidate lines with no blank lines or surrounding envelope. Return candidate text only, with no JSON, labels, Markdown, or commentary.", MaxRepositoryRequirementInventoryCandidates),
		"REPOSITORY REQUIREMENT INVENTORY INPUT:\n" + projection,
		"FINAL QUESTION:\nWhat bounded source-ordered candidate-clause inventory is explicitly grounded in the request? Return only one exact source quote per non-empty line.",
	}, "\n\n"), nil
}

func DecodeRepositoryRequirementInventory(
	input RepositoryRequirementInterpretationInput,
	raw string,
) (RepositoryRequirementInventory, error) {
	var zero RepositoryRequirementInventory
	if err := input.validate(); err != nil {
		return zero, err
	}
	inventoryText, err := decodeRawSemanticLeaf(
		"repository requirement inventory",
		raw,
		maxRepositoryRequirementInventoryBytes,
		true,
	)
	if err != nil {
		return zero, err
	}
	if strings.ContainsRune(inventoryText, '\r') {
		return zero, fmt.Errorf("repository requirement inventory must use LF line boundaries")
	}
	candidates := strings.Split(inventoryText, "\n")
	if len(candidates) < 1 || len(candidates) > MaxRepositoryRequirementInventoryCandidates {
		return zero, fmt.Errorf(
			"repository requirement inventory must contain between 1 and %d candidate lines",
			MaxRepositoryRequirementInventoryCandidates,
		)
	}
	for index, candidate := range candidates {
		leaf, err := decodeRawSemanticLeaf(
			fmt.Sprintf("repository requirement inventory candidate %d", index),
			candidate,
			maxRequirementQuoteBytes,
			false,
		)
		if err != nil {
			return zero, err
		}
		candidates[index] = leaf
	}
	authoritySHA256, err := repositoryRequirementInventoryAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := RepositoryRequirementInventory{
		Schema:          RepositoryRequirementInventorySchemaV1,
		AuthoritySHA256: authoritySHA256,
		RawSHA256:       ExactObjectiveContextSHA(strings.Join(candidates, "\n")),
		Candidates:      append([]string(nil), candidates...),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (inventory RepositoryRequirementInventory) ValidateFor(
	input RepositoryRequirementInterpretationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if inventory.Schema != RepositoryRequirementInventorySchemaV1 {
		return fmt.Errorf(
			"repository requirement inventory schema must be %q",
			RepositoryRequirementInventorySchemaV1,
		)
	}
	authoritySHA256, err := repositoryRequirementInventoryAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if inventory.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("repository requirement inventory authority hash does not match")
	}
	if inventory.Candidates == nil || len(inventory.Candidates) < 1 ||
		len(inventory.Candidates) > MaxRepositoryRequirementInventoryCandidates {
		return fmt.Errorf(
			"repository requirement inventory must contain between 1 and %d candidates",
			MaxRepositoryRequirementInventoryCandidates,
		)
	}
	previousStart := -1
	for index, candidate := range inventory.Candidates {
		if strings.ContainsAny(candidate, "\r\n") {
			return fmt.Errorf("repository requirement inventory candidate %d must be one line", index)
		}
		if err := validateRepositoryRequirementStatement(
			fmt.Sprintf("repository requirement inventory candidate %d", index),
			candidate,
		); err != nil {
			return err
		}
		span, err := uniqueTextSpan(input.UserRequest, candidate)
		if err != nil {
			return fmt.Errorf(
				"repository requirement inventory candidate %d %q: %w",
				index,
				candidate,
				err,
			)
		}
		if span.Start < previousStart {
			return fmt.Errorf("repository requirement inventory candidates must preserve source order")
		}
		previousStart = span.Start
	}
	raw := strings.Join(inventory.Candidates, "\n")
	if inventory.RawSHA256 != ExactObjectiveContextSHA(raw) {
		return fmt.Errorf("repository requirement inventory raw hash does not match")
	}
	return nil
}

func repositoryRequirementInventoryAuthoritySHA256(
	input RepositoryRequirementInterpretationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode repository requirement inventory authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
