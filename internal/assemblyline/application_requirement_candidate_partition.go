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
		"Return one complete lossless proper refinement of the exact compound candidate into strictly narrower semantic children.",
		"Treat the supplied compound relation as fixed. Preserve every explicit meaning exactly once; omit nothing and add nothing.",
		"Every child must be strictly narrower than and byte-different from the parent. Return only the distinct semantic child text; omit parent restatements, summaries, and umbrella clauses.",
		"Do not propose alternative implementations, technologies, architectures, APIs, algorithms, storage mechanisms, tools, or multiple ways to satisfy the parent. Do not repeat the parent with added implementation details or added using-clauses.",
	}
	if input.Kind != nil {
		return strings.Join(append(common,
			"Return exactly two raw child lines. The first line must contain all and only the task-local runtime-outcome meaning already asserted by the parent. Express it as one complete declarative runtime outcome with an explicit running-software or user subject; exclude build wording, delivery surface, language, framework, and toolchain. The second line must contain all and only the non-runtime constraint meaning already asserted by the parent.",
			"A behavior-denoting product or category noun may be restated only as the core runtime action or governed result it literally denotes. Do not add customary controls, variants, inputs, outputs, or features.",
			"Return child text only, with no JSON, labels, Markdown, commentary, or blank lines.",
			"EXACT COMPOUND CANDIDATE:\n"+input.Candidate,
			"EXACT COMPOUND RELATION:\n"+ApplicationRequirementCandidateMixed,
			"FINAL QUESTION:\nWhat are the exact runtime component and exact non-runtime component? Return exactly those two raw child lines, runtime first and non-runtime second.",
		), "\n\n"), nil
	}
	return strings.Join(append(common,
		"Keep one runtime outcome's operands, condition, determining rule, and resulting output together when they jointly describe that outcome.",
		fmt.Sprintf("Return between 2 and %d raw child lines, one complete runtime outcome per line, with no blank lines or surrounding envelope. Return child text only, with no JSON, labels, Markdown, or commentary.", MaxApplicationRequirementCandidatePartitionLeaves),
		"EXACT COMPOUND CANDIDATE:\n"+input.Candidate,
		"EXACT COMPOUND RELATION:\n"+ApplicationRequirementMultipleRuntimeOutcomes,
		"FINAL QUESTION:\nWhat is the complete source-ordered partition? Return only one raw child candidate per non-empty line.",
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
