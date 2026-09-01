package assemblyline

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	WorkRoleplayCanonFactInventory              WorkKind = "roleplay_canon_fact_inventory"
	WorkRoleplayCanonFactCandidateAuthorization WorkKind = "roleplay_canon_fact_candidate_authorization"
	WorkRoleplayCanonFactCandidateRelation      WorkKind = "roleplay_canon_fact_candidate_relation"

	RoleplayCanonFactEstablished    = "ESTABLISHED_BY_CURRENT_CONTRIBUTION"
	RoleplayCanonFactNotEstablished = "NOT_ESTABLISHED_BY_CURRENT_CONTRIBUTION"

	RoleplayCanonFactsEquivalent = "SAME_CANON_FACT"
	RoleplayCanonFactsDistinct   = "DISTINCT_CANON_FACT"

	RoleplayCanonFactInventorySchemaV1     = "omnidex.roleplay-canon-fact-inventory.v1"
	RoleplayCanonFactAuthorizationSchemaV1 = "omnidex.roleplay-canon-fact-authorization.v1"
	RoleplayCanonFactRelationSchemaV1      = "omnidex.roleplay-canon-fact-relation.v1"

	maxRoleplayCanonFactInventoryBytes = MaxRoleplayCanonFactsPerTurn*roleplay.MaxCanonEventBytes + MaxRoleplayCanonFactsPerTurn - 1
)

type RoleplayCanonFactInventory struct {
	Schema          string   `json:"schema"`
	AuthoritySHA256 string   `json:"authority_sha256"`
	RawSHA256       string   `json:"raw_sha256"`
	Candidates      []string `json:"candidates"`
}

func NewRoleplayCanonFactInventoryJob(input RoleplayCanonExtractionInput) (PortableJob, error) {
	return newPortableJob(WorkRoleplayCanonFactInventory, input)
}

func BuildRoleplayCanonFactInventoryPrompt(input RoleplayCanonExtractionInput) (string, error) {
	authority, err := renderRoleplayCanonExtractionAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"What durable fictional facts are directly established by this contribution?",
		"Treat the current contribution as the only fact source. Established fictional context is reference material, and an earlier user contribution may only resolve references in an in-character response. Questions, requests, and directions are not fictional events.",
		"Exclude implications, restatements of established context, inferred character visibility, decorative sensory detail, and real-world claims. Attribute first-person statements, actions, possessions, and knowledge only to " + strconv.Quote(input.Source.AttributedPersonaName) + ".",
		fmt.Sprintf("List between 1 and %d concise standalone facts in contribution source order, one fact per line. Do not merge distinct facts, add customary detail, or infer a future event.", MaxRoleplayCanonFactsPerTurn),
		authority,
	}, "\n\n"), nil
}

func DecodeRoleplayCanonFactInventory(
	input RoleplayCanonExtractionInput,
	raw string,
) (RoleplayCanonFactInventory, error) {
	var zero RoleplayCanonFactInventory
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"roleplay canon fact inventory", raw, maxRoleplayCanonFactInventoryBytes, true,
	)
	if err != nil {
		return zero, err
	}
	if strings.ContainsRune(leaf, '\r') {
		return zero, fmt.Errorf("roleplay canon fact inventory must use LF line boundaries")
	}
	candidates := strings.Split(leaf, "\n")
	if len(candidates) < 1 || len(candidates) > MaxRoleplayCanonFactsPerTurn {
		return zero, fmt.Errorf(
			"roleplay canon fact inventory must contain 1..%d candidates",
			MaxRoleplayCanonFactsPerTurn,
		)
	}
	for index, candidate := range candidates {
		if err := roleplay.ValidateCanonFact(candidate); err != nil {
			return zero, fmt.Errorf("roleplay canon fact inventory candidate %d: %w", index, err)
		}
	}
	authoritySHA256, err := roleplayCanonSemanticAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := RoleplayCanonFactInventory{
		Schema:          RoleplayCanonFactInventorySchemaV1,
		AuthoritySHA256: authoritySHA256,
		RawSHA256:       ExactObjectiveContextSHA(leaf),
		Candidates:      append([]string{}, candidates...),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (inventory RoleplayCanonFactInventory) ValidateFor(input RoleplayCanonExtractionInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if inventory.Schema != RoleplayCanonFactInventorySchemaV1 {
		return fmt.Errorf("roleplay canon fact inventory schema must be %q", RoleplayCanonFactInventorySchemaV1)
	}
	authoritySHA256, err := roleplayCanonSemanticAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if inventory.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("roleplay canon fact inventory authority hash does not match")
	}
	if len(inventory.Candidates) < 1 || len(inventory.Candidates) > MaxRoleplayCanonFactsPerTurn {
		return fmt.Errorf("roleplay canon fact inventory must contain 1..%d candidates", MaxRoleplayCanonFactsPerTurn)
	}
	for index, candidate := range inventory.Candidates {
		if strings.ContainsAny(candidate, "\r\n") {
			return fmt.Errorf("roleplay canon fact inventory candidate %d must be one line", index)
		}
		if err := roleplay.ValidateCanonFact(candidate); err != nil {
			return fmt.Errorf("roleplay canon fact inventory candidate %d: %w", index, err)
		}
	}
	raw := strings.Join(inventory.Candidates, "\n")
	if inventory.RawSHA256 != ExactObjectiveContextSHA(raw) {
		return fmt.Errorf("roleplay canon fact inventory raw hash does not match")
	}
	return nil
}

func AssembleRoleplayCanonExtractionDecision(
	input RoleplayCanonExtractionInput,
	facts []string,
) (RoleplayCanonExtractionDecision, error) {
	decision := RoleplayCanonExtractionDecision{
		Schema: RoleplayCanonExtractionSchemaV1,
		Facts:  append([]string{}, facts...),
	}
	return decision.ResolveFor(input)
}

func renderRoleplayCanonExtractionAuthority(input RoleplayCanonExtractionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	contextText, err := renderObjectiveContextForModel(input.Context)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, 3)
	if input.Source.Kind == RoleplayCanonSourceUserContribution {
		description, err := describeRoleplayCanonUserContribution(
			input.Source.PersonaKind,
			input.Source.ContributionKind,
		)
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf(
			"Current %s by %s:\n%s",
			description,
			strconv.Quote(input.Source.AttributedPersonaName),
			input.Source.ExactContribution,
		))
	} else {
		parts = append(parts, fmt.Sprintf(
			"Current in-character response by %s:\n%s",
			strconv.Quote(input.Source.AttributedPersonaName),
			input.Source.ExactContribution,
		))
	}
	if input.AntecedentUserTurn != nil {
		antecedent := input.AntecedentUserTurn
		description, err := describeRoleplayCanonUserContribution(
			antecedent.PersonaKind,
			antecedent.ContributionKind,
		)
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf(
			"Earlier %s by %s, supplied only to resolve references in the response:\n%s",
			description,
			strconv.Quote(antecedent.PersonaName),
			antecedent.ContributionContext,
		))
	}
	if contextText != "" {
		parts = append(parts, "Established fictional context:\n"+contextText)
	}
	authority := strings.Join(parts, "\n\n")
	if len(authority) > maxPortablePayloadBytes {
		return "", fmt.Errorf("roleplay canon fact authority exceeds %d bytes", maxPortablePayloadBytes)
	}
	return authority, nil
}

func describeRoleplayCanonUserContribution(
	persona roleplay.UserPersonaKind,
	contribution roleplay.UserContributionKind,
) (string, error) {
	personaText := ""
	switch persona {
	case roleplay.UserPersonaCharacter:
		personaText = "character contribution"
	case roleplay.UserPersonaNarrator:
		personaText = "narrator contribution"
	default:
		return "", fmt.Errorf("roleplay canon persona kind %q is unsupported", persona)
	}
	contributionText := ""
	switch contribution {
	case roleplay.UserContributionDialogue:
		contributionText = "spoken dialogue"
	case roleplay.UserContributionAction:
		contributionText = "a described action"
	case roleplay.UserContributionActionDialogue:
		contributionText = "a described action and spoken dialogue"
	case roleplay.UserContributionNarration:
		contributionText = "narration"
	case roleplay.UserContributionDirection:
		contributionText = "a direction"
	case roleplay.UserContributionNarrationDirection:
		contributionText = "narration containing a direction"
	case roleplay.UserContributionStructured:
		contributionText = "structured action, dialogue, or events"
	case roleplay.UserContributionCommand:
		contributionText = "a command"
	default:
		return "", fmt.Errorf("roleplay canon contribution kind %q is unsupported", contribution)
	}
	return personaText + " consisting of " + contributionText, nil
}

func roleplayCanonSemanticAuthoritySHA256(value any) (string, error) {
	authority, err := exactjson.Canonical(value)
	if err != nil {
		return "", fmt.Errorf("encode roleplay canon semantic authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
