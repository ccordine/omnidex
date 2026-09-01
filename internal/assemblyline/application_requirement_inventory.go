package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
)

const (
	WorkApplicationRequirementInventory WorkKind = "application_requirement_inventory"

	ApplicationNoRuntimeRequirementCandidates = "NO_RUNTIME_REQUIREMENT_CANDIDATES"

	MaxApplicationRequirementInventoryCandidates = MaxApplicationRequirementLeaves * 3
	maxApplicationRequirementInventoryBytes      = MaxApplicationRequirementInventoryCandidates*maxRequirementQuoteBytes +
		MaxApplicationRequirementInventoryCandidates - 1

	ApplicationRequirementInventorySchemaV5 = "omnidex.application-requirement-inventory.v5"
)

type ApplicationRequirementInventoryInput struct {
	UserRequest string                `json:"user_request"`
	Context     ApplicationContext    `json:"context"`
	ScopeMode   model.CodingScopeMode `json:"scope_mode"`
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
	return newPortableJob(
		WorkApplicationRequirementInventory,
		input,
	)
}

func (input ApplicationRequirementInventoryInput) validate() error {
	if err := (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate(); err != nil {
		return err
	}
	return input.ScopeMode.Validate()
}

func BuildApplicationRequirementInventoryPrompt(
	input ApplicationRequirementInventoryInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := renderApplicationContextModelProjection(input.UserRequest, input.Context)
	scopeGuidance, err := applicationRequirementInventoryScopeGuidance(input.ScopeMode)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		fmt.Sprintf("What atomic finished-software runtime outcomes would constitute useful work toward satisfying this request? List one independent outcome per line, up to %d. If there are none, answer %s.", MaxApplicationRequirementInventoryCandidates, ApplicationNoRuntimeRequirementCandidates),
		scopeGuidance,
		"Generate distinct semantic work or runtime outcomes, not speculative alternative implementation mechanisms. Each candidate must state exactly one independently testable runtime outcome: one behavior, user-visible element, observable quality, state or persistence behavior, or runtime data or output requirement. Different mechanisms for realizing the same outcome are not separate candidates. Split only independent outcomes; never enumerate modes, variants, cases, algorithms, or alternative ways to perform the same outcome.",
		"The ordinary meaning of a purpose-denoting product or category name is request content. For each such named purpose, include one minimal end-to-end candidate containing only the literal core operation or governed result inherent in that name. Express the purpose noun as the simplest corresponding action and governed object. Do not add an input, parameter, criterion, destination, mechanism, interface, trigger, or qualifying phrase unless the request states it.",
		"When the literal purpose is to transform, read, extract, decode, calculate, or otherwise derive a governed value, state the minimal independently verifiable governed result instead of a bare activity. Express the value produced by the operation at the abstraction inherent in the purpose. Verifiable does not authorize a presentation or delivery channel: never add display, show, render, return, download, store, transmit, notify, an interface, an output format, or any other unstated mechanism. Never invent the result's format, algorithm, defaults, quality rules, or limits.",
		"A delivery surface or construction technology does not imply a runtime mechanism, interface, input source, or interaction. Do not add one unless the immutable request states it.",
		"Preserve the actor, action, governed object, modality, determining relation, and resulting observation that jointly define an outcome. Keep the software as the semantic subject of each capability outcome. If the request says the software lets, allows, or enables an actor to act, preserve that software-provided ability rather than asserting that the actor necessarily performs the action. An ability, permission, possibility, or enablement must remain that relation rather than becoming an assertion that the action necessarily occurs. State the runtime outcome itself; a topic, title, noun phrase, or feature label is not a runtime outcome. Preserve every genuinely separate runtime outcome as its own candidate. Do not duplicate, paraphrase, restate, or split the same core outcome into multiple candidates; a more explicit statement already represents its named core outcome.",
		"The maximum candidate count is a safety ceiling, never a generation target. When the request has only one distinct runtime outcome, preserve it as one candidate rather than inventing multiple ways to implement it. When the request already states that atomic runtime outcome, that stated outcome is the candidate rather than a summary, abstraction, or generalized restatement.",
		"Do not add a generic trigger frame such as 'when invoked' or 'on request' unless the immutable request states that trigger.",
		"Exclude bare product identity, build or create directions, delivery surface, language, framework, toolchain, packaging, testing, deployment, and other construction constraints. Do not turn customary implementation mechanisms or process steps into runtime outcomes.",
		projection,
	}, "\n\n"), nil
}

func applicationRequirementInventoryScopeGuidance(mode model.CodingScopeMode) (string, error) {
	switch mode {
	case model.CodingScopeModeStrict:
		return "Include only outcomes directly grounded in the request and established facts. Do not add optional or derived product scope.", nil
	case model.CodingScopeModeNormal:
		return "Include directly stated outcomes and ordinary necessary or useful consequences reasonably justified by the objective or established facts. A useful consequence need not repeat the user's literal wording. Exclude unrelated or merely optional speculative enhancements.", nil
	case model.CodingScopeModeExpansive:
		return "Include directly stated outcomes, ordinary justified consequences, and cohesive objective-aligned possibilities that could usefully expand the objective. Do not include work that conflicts with the request or established facts or materially departs into an unrelated objective.", nil
	default:
		return "", fmt.Errorf("application requirement inventory scope mode %q is unsupported", mode)
	}
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
		Schema:          ApplicationRequirementInventorySchemaV5,
		AuthoritySHA256: authoritySHA256,
		RawSHA256:       ExactObjectiveContextSHA(applicationRequirementInventoryRaw(candidates)),
		Candidates:      append([]string{}, candidates...),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func applicationRequirementInventoryRaw(candidates []string) string {
	if len(candidates) == 0 {
		return ApplicationNoRuntimeRequirementCandidates
	}
	return strings.Join(candidates, "\n")
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
		if leaf == ApplicationNoRuntimeRequirementCandidates {
			return nil, "", fmt.Errorf(
				"%s candidate %d cannot mix the registered absence result with positive candidates",
				label,
				index,
			)
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
	if inventory.Schema != ApplicationRequirementInventorySchemaV5 {
		return fmt.Errorf(
			"application requirement inventory schema must be %q",
			ApplicationRequirementInventorySchemaV5,
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
		if candidate == ApplicationNoRuntimeRequirementCandidates {
			return fmt.Errorf(
				"application requirement inventory candidate %d cannot be the registered absence result",
				index,
			)
		}
		if err := validateApplicationIntentText(
			"requirement inventory candidate",
			candidate,
			maxRequirementQuoteBytes,
		); err != nil {
			return fmt.Errorf("application requirement inventory candidate %d: %w", index, err)
		}
	}
	raw := applicationRequirementInventoryRaw(inventory.Candidates)
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
