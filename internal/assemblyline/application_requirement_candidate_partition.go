package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationRequirementCandidatePartition WorkKind = "application_requirement_candidate_partition"

	ApplicationRequirementCandidatePartitionSchemaV1  = "omnidex.application-requirement-candidate-partition.v1"
	MaxApplicationRequirementCandidatePartitionLeaves = MaxApplicationRequirementLeaves
	MaxApplicationRequirementCandidatePartitionDepth  = 3
	MaxApplicationRequirementCandidateQueueNodes      = MaxApplicationRequirementInventoryCandidates *
		(1 + MaxApplicationRequirementCandidatePartitionLeaves +
			MaxApplicationRequirementCandidatePartitionLeaves*MaxApplicationRequirementCandidatePartitionLeaves +
			MaxApplicationRequirementCandidatePartitionLeaves*MaxApplicationRequirementCandidatePartitionLeaves*
				MaxApplicationRequirementCandidatePartitionLeaves)
	maxApplicationRequirementCandidatePartitionBytes = MaxApplicationRequirementCandidatePartitionLeaves*maxRequirementQuoteBytes +
		MaxApplicationRequirementCandidatePartitionLeaves - 1
)

type ApplicationRequirementCandidatePartitionInput struct {
	Candidate   string                                            `json:"candidate"`
	Kind        *ApplicationRequirementCandidateKindResult        `json:"kind,omitempty"`
	Cardinality *ApplicationRequirementCandidateCardinalityResult `json:"cardinality,omitempty"`
}

type ApplicationRequirementCandidatePartition struct {
	Schema          string   `json:"schema"`
	AuthoritySHA256 string   `json:"authority_sha256"`
	RawSHA256       string   `json:"raw_sha256"`
	Candidates      []string `json:"candidates"`
}

func NewApplicationRequirementCandidatePartitionJob(
	input ApplicationRequirementCandidatePartitionInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkApplicationRequirementCandidatePartition,
		input,
	)
}

func (input ApplicationRequirementCandidatePartitionInput) validate() error {
	if err := validateApplicationIntentText(
		"application requirement partition candidate",
		input.Candidate,
		maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	if (input.Kind == nil) == (input.Cardinality == nil) {
		return fmt.Errorf(
			"application requirement candidate partition requires exactly one mixed-kind or multi-outcome receipt",
		)
	}
	if input.Kind != nil {
		kindInput := ApplicationRequirementCandidateKindInput{Candidate: input.Candidate}
		if err := input.Kind.ValidateFor(kindInput); err != nil {
			return fmt.Errorf("validate partition kind receipt: %w", err)
		}
		if input.Kind.Relation != ApplicationRequirementCandidateMixed {
			return fmt.Errorf(
				"application requirement candidate partition kind must be %q",
				ApplicationRequirementCandidateMixed,
			)
		}
		return nil
	}
	cardinalityInput := ApplicationRequirementCandidateCardinalityInput{
		Candidate: input.Candidate,
	}
	if err := input.Cardinality.ValidateFor(cardinalityInput); err != nil {
		return fmt.Errorf("validate partition cardinality receipt: %w", err)
	}
	if input.Cardinality.Relation != ApplicationRequirementMultipleRuntimeOutcomes {
		return fmt.Errorf(
			"application requirement candidate partition cardinality must be %q",
			ApplicationRequirementMultipleRuntimeOutcomes,
		)
	}
	return nil
}

func BuildApplicationRequirementCandidatePartitionPrompt(
	input ApplicationRequirementCandidatePartitionInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	common := []string{
		"Identify the complete lossless partition of this compound requirement into narrower semantic statements.",
		"Include every explicit meaning exactly once; omit nothing and add nothing. Each statement must be narrower than the compound requirement, not a parent restatement, summary, or umbrella clause.",
		"Exclude alternative implementations, technologies, architectures, APIs, algorithms, storage mechanisms, tools, multiple ways to satisfy the parent, and parent restatements with added implementation details or using-clauses.",
	}
	if input.Kind != nil {
		return strings.Join(append(common,
			"Write two statements on separate lines: first the complete task-local runtime outcome, then the complete construction or delivery constraint. The first statement needs an explicit running-software or user subject and excludes build wording, delivery surface, language, framework, and toolchain.",
			"A behavior-denoting product or category noun may be restated only as the core runtime action or governed result it literally denotes. Do not add customary controls, variants, inputs, outputs, or features.",
			"Compound requirement:\n"+input.Candidate,
		), "\n\n"), nil
	}
	return strings.Join(append(common,
		"Keep one runtime outcome's operands, condition, determining rule, and resulting output together when they jointly describe that outcome.",
		fmt.Sprintf("Write the source-ordered partition as 2 to %d complete runtime outcomes, one statement per line.", MaxApplicationRequirementCandidatePartitionLeaves),
		"Compound requirement:\n"+input.Candidate,
	), "\n\n"), nil
}

func DecodeApplicationRequirementCandidatePartition(
	input ApplicationRequirementCandidatePartitionInput,
	raw string,
) (ApplicationRequirementCandidatePartition, error) {
	var zero ApplicationRequirementCandidatePartition
	if err := input.validate(); err != nil {
		return zero, err
	}
	minimum, maximum := applicationRequirementCandidatePartitionBounds(input)
	children, normalized, err := decodeApplicationRequirementCandidateLines(
		"application requirement candidate partition",
		raw,
		minimum,
		maximum,
	)
	if err != nil {
		return zero, err
	}
	seen := map[string]struct{}{input.Candidate: {}}
	for index, child := range children {
		if _, duplicate := seen[child]; duplicate {
			return zero, fmt.Errorf(
				"application requirement candidate partition child %d repeats its parent or sibling",
				index,
			)
		}
		seen[child] = struct{}{}
	}
	authoritySHA256, err := applicationRequirementCandidatePartitionAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCandidatePartition{
		Schema:          ApplicationRequirementCandidatePartitionSchemaV1,
		AuthoritySHA256: authoritySHA256,
		RawSHA256:       ExactObjectiveContextSHA(normalized),
		Candidates:      append([]string(nil), children...),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (partition ApplicationRequirementCandidatePartition) ValidateFor(
	input ApplicationRequirementCandidatePartitionInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if partition.Schema != ApplicationRequirementCandidatePartitionSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate partition schema must be %q",
			ApplicationRequirementCandidatePartitionSchemaV1,
		)
	}
	authoritySHA256, err := applicationRequirementCandidatePartitionAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if partition.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application requirement candidate partition authority hash does not match")
	}
	minimum, maximum := applicationRequirementCandidatePartitionBounds(input)
	if len(partition.Candidates) < minimum || len(partition.Candidates) > maximum {
		return fmt.Errorf(
			"application requirement candidate partition must contain between %d and %d children",
			minimum,
			maximum,
		)
	}
	seen := map[string]struct{}{input.Candidate: {}}
	for index, child := range partition.Candidates {
		if strings.ContainsAny(child, "\r\n") {
			return fmt.Errorf("application requirement candidate partition child %d must be one line", index)
		}
		if err := validateApplicationIntentText(
			"application requirement partition child",
			child,
			maxRequirementQuoteBytes,
		); err != nil {
			return fmt.Errorf("application requirement candidate partition child %d: %w", index, err)
		}
		if _, duplicate := seen[child]; duplicate {
			return fmt.Errorf(
				"application requirement candidate partition child %d repeats its parent or sibling",
				index,
			)
		}
		seen[child] = struct{}{}
	}
	raw := strings.Join(partition.Candidates, "\n")
	if partition.RawSHA256 != ExactObjectiveContextSHA(raw) {
		return fmt.Errorf("application requirement candidate partition raw hash does not match")
	}
	return nil
}

func applicationRequirementCandidatePartitionBounds(
	input ApplicationRequirementCandidatePartitionInput,
) (int, int) {
	if input.Kind != nil {
		return 2, 2
	}
	return 2, MaxApplicationRequirementCandidatePartitionLeaves
}

func applicationRequirementCandidatePartitionAuthoritySHA256(
	input ApplicationRequirementCandidatePartitionInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application requirement candidate partition authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
