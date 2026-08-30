package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationRequirementInventory WorkKind = "application_requirement_inventory"

	ApplicationNoRuntimeRequirementCandidates = "NO_RUNTIME_REQUIREMENT_CANDIDATES"

	MaxApplicationRequirementInventoryCandidates = MaxApplicationRequirementLeaves * 3
	maxApplicationRequirementInventoryBytes      = MaxApplicationRequirementInventoryCandidates*maxRequirementQuoteBytes +
		MaxApplicationRequirementInventoryCandidates - 1

	ApplicationRequirementInventorySchemaV4 = "omnidex.application-requirement-inventory.v4"
)

type ApplicationRequirementInventoryInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
}

// ApplicationRequirementInventory is one bounded, untrusted generation of
// candidate runtime leaves. Code owns authorization, classification,
// partitioning, duplicate removal, retention, and queue exhaustion.
type ApplicationRequirementInventory struct {
	Schema          string   `json:"schema"`
	AuthoritySHA256 string   `json:"authority_sha256"`
	RawSHA256       string   `json:"raw_sha256"`
	Candidates      []string `json:"candidates"`
}

func NewApplicationRequirementInventoryJob(
	input ApplicationRequirementInventoryInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementInventory,
		input,
		input.validate,
	)
}

func (input ApplicationRequirementInventoryInput) validate() error {
	if err := (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate(); err != nil {
		return err
	}
	return nil
}

func BuildApplicationRequirementInventoryPrompt(
	input ApplicationRequirementInventoryInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := renderApplicationContextModelProjection(input.UserRequest, input.Context)
	return strings.Join([]string{
		"Return one bounded source-ordered inventory of atomic finished-software runtime-outcome candidates required by the immutable software request.",
		fmt.Sprintf("Return exactly %s when the request grounds no runtime-outcome candidate. Otherwise return between 1 and %d positive candidate lines, one per independent runtime outcome.", ApplicationNoRuntimeRequirementCandidates, MaxApplicationRequirementInventoryCandidates),
		"This is minimal semantic extraction, not brainstorming. Each candidate must state exactly one independently testable runtime outcome: one behavior, user-visible element, observable quality, state or persistence behavior, or runtime data or output requirement. Split only independent outcomes; never enumerate modes, variants, cases, algorithms, optional features, or alternative ways to perform the same outcome.",
		"The ordinary meaning of a purpose-denoting product or category name is request content. For each such named purpose, return exactly one minimal end-to-end candidate containing only the literal core operation or governed result inherent in that name. If one named purpose is the request's only runtime meaning, return exactly one candidate line. Express the purpose noun as the simplest corresponding action and governed object. Do not add an input, parameter, criterion, destination, mechanism, interface, trigger, or qualifying phrase unless the request states it.",
		"When the literal purpose is to transform, read, extract, decode, calculate, or otherwise derive a governed value, state the minimal independently verifiable governed result instead of a bare activity. Express the value produced by the operation at the abstraction inherent in the purpose. Verifiable does not authorize a presentation or delivery channel: never add display, show, render, return, download, store, transmit, notify, an interface, an output format, or any other unstated mechanism. Never invent the result's format, algorithm, defaults, quality rules, or limits.",
		"A delivery surface or construction technology does not imply a runtime mechanism, interface, input source, or interaction. Do not add one unless the immutable request states it.",
		"Keep the inputs or trigger, determining relation, and resulting observation together when they jointly define one outcome. Preserve every separately stated runtime outcome, but do not repeat a named core outcome when a more explicit statement already represents it.",
		"Do not add a generic trigger frame such as 'when invoked' or 'on request' unless the immutable request states that trigger.",
		"Do not return bare product identity, build or create directions, delivery surface, language, framework, toolchain, packaging, testing, deployment, or other construction constraints. Do not invent customary controls, history, persistence, process steps, enhancements, prerequisites, or implementation choices.",
		"Every positive candidate line must begin exactly with 'The finished software ' and continue with one complete runtime outcome. Order candidates by the first request meaning that grounds each one; do not duplicate a candidate. The maximum is a safety bound, not a target.",
		fmt.Sprintf("Return at most %d candidates, one complete raw candidate per non-empty line. Return only the permitted absence value or candidate lines, with no JSON, labels, numbering, bullets, Markdown, commentary, or surrounding envelope.", MaxApplicationRequirementInventoryCandidates),
		"APPLICATION REQUIREMENT INVENTORY INPUT:\n" + projection,
		"FINAL QUESTION:\nWhat bounded atomic finished-software runtime-outcome candidate inventory is grounded by this request? Return only the exact permitted raw absence value or candidate lines.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementInventory(
	input ApplicationRequirementInventoryInput,
	raw string,
) (ApplicationRequirementInventory, error) {
	var zero ApplicationRequirementInventory
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement inventory",
		raw,
		max(maxApplicationRequirementInventoryBytes, len(ApplicationNoRuntimeRequirementCandidates)),
		true,
	)
	if err != nil {
		return zero, err
	}
	candidates := []string{}
	if leaf != ApplicationNoRuntimeRequirementCandidates {
		if strings.Contains(
			strings.ToUpper(leaf),
			ApplicationNoRuntimeRequirementCandidates,
		) {
			return zero, fmt.Errorf(
				"application requirement inventory absence value must be returned exactly",
			)
		}
		candidates, _, err = decodeApplicationRequirementCandidateLines(
			"application requirement inventory",
			leaf,
			1,
			MaxApplicationRequirementInventoryCandidates,
		)
		if err != nil {
			return zero, err
		}
	}
	authoritySHA256, err := applicationRequirementInventoryAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementInventory{
		Schema:          ApplicationRequirementInventorySchemaV4,
		AuthoritySHA256: authoritySHA256,
		RawSHA256:       ExactObjectiveContextSHA(leaf),
		Candidates:      append([]string{}, candidates...),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func decodeApplicationRequirementCandidateLines(
	label string,
	raw string,
	minimum int,
	maximum int,
) ([]string, string, error) {
	if minimum < 1 || maximum < minimum {
		return nil, "", fmt.Errorf("%s line bounds are invalid", label)
	}
	if strings.ContainsRune(raw, '\r') {
		return nil, "", fmt.Errorf("%s must use LF line boundaries", label)
	}
	candidates := strings.Split(raw, "\n")
	if len(candidates) < minimum || len(candidates) > maximum {
		return nil, "", fmt.Errorf(
			"%s must contain between %d and %d candidate lines",
			label,
			minimum,
			maximum,
		)
	}
	for index, candidate := range candidates {
		leaf, err := decodeRawSemanticLeaf(
			fmt.Sprintf("%s candidate %d", label, index),
			candidate,
			maxRequirementQuoteBytes,
			false,
		)
		if err != nil {
			return nil, "", err
		}
		if err := validateApplicationIntentText(
			label+" candidate",
			leaf,
			maxRequirementQuoteBytes,
		); err != nil {
			return nil, "", fmt.Errorf("%s candidate %d: %w", label, index, err)
		}
		candidates[index] = leaf
	}
	return append([]string(nil), candidates...), strings.Join(candidates, "\n"), nil
}

func (inventory ApplicationRequirementInventory) ValidateFor(
	input ApplicationRequirementInventoryInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if inventory.Schema != ApplicationRequirementInventorySchemaV4 {
		return fmt.Errorf(
			"application requirement inventory schema must be %q",
			ApplicationRequirementInventorySchemaV4,
		)
	}
	authoritySHA256, err := applicationRequirementInventoryAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if inventory.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application requirement inventory authority hash does not match")
	}
	if inventory.Candidates == nil {
		return fmt.Errorf(
			"application requirement inventory candidates must be an array",
		)
	}
	if len(inventory.Candidates) > MaxApplicationRequirementInventoryCandidates {
		return fmt.Errorf(
			"application requirement inventory must contain at most %d candidates",
			MaxApplicationRequirementInventoryCandidates,
		)
	}
	for index, candidate := range inventory.Candidates {
		if strings.ContainsAny(candidate, "\r\n") {
			return fmt.Errorf("application requirement inventory candidate %d must be one line", index)
		}
		if err := validateApplicationIntentText(
			"requirement inventory candidate",
			candidate,
			maxRequirementQuoteBytes,
		); err != nil {
			return fmt.Errorf("application requirement inventory candidate %d: %w", index, err)
		}
	}
	raw := ApplicationNoRuntimeRequirementCandidates
	if len(inventory.Candidates) > 0 {
		raw = strings.Join(inventory.Candidates, "\n")
	}
	if inventory.RawSHA256 != ExactObjectiveContextSHA(raw) {
		return fmt.Errorf("application requirement inventory raw hash does not match")
	}
	return nil
}

func applicationRequirementInventoryAuthoritySHA256(
	input ApplicationRequirementInventoryInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application requirement inventory authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
