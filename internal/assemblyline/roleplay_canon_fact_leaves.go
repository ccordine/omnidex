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

	RoleplayNoCanonFactCandidates = "NO_CANON_FACT_CANDIDATES"

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
	return newValidatedPortableJob(WorkRoleplayCanonFactInventory, input, input.validate)
}

func BuildRoleplayCanonFactInventoryPrompt(input RoleplayCanonExtractionInput) (string, error) {
	authority, err := renderRoleplayCanonExtractionAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return one bounded source-ordered inventory of candidate durable fictional facts directly established by the exact current contribution.",
		"Treat only the exact contribution as the fact source. Established context is reference material, and an antecedent user turn only resolves references in an assistant contribution. Questions, requests, and directions are not fictional events.",
		"Exclude implications, restatements of established context, inferred character visibility, decorative sensory detail, and real-world claims. Attribute first-person statements, actions, possessions, and knowledge only to " + strconv.Quote(input.Source.AttributedPersonaName) + ".",
		fmt.Sprintf("Return at most %d candidates, one concise standalone candidate fact per non-empty raw line in contribution source order. Do not merge distinct facts, add customary detail, or infer a future event.", MaxRoleplayCanonFactsPerTurn),
		"When the contribution directly establishes no durable fictional fact, return only NO_CANON_FACT_CANDIDATES. Otherwise return candidate text only, with no JSON, labels, Markdown, explanation, or surrounding envelope.",
		"ROLEPLAY CANON CANDIDATE INVENTORY AUTHORITY:\n" + authority,
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
	candidates := []string{}
	if leaf != RoleplayNoCanonFactCandidates {
		if strings.ContainsRune(leaf, '\r') {
			return zero, fmt.Errorf("roleplay canon fact inventory must use LF line boundaries")
		}
		candidates = strings.Split(leaf, "\n")
		if len(candidates) > MaxRoleplayCanonFactsPerTurn {
			return zero, fmt.Errorf(
				"roleplay canon fact inventory must contain 0..%d candidates",
				MaxRoleplayCanonFactsPerTurn,
			)
		}
		for index, candidate := range candidates {
			if err := roleplay.ValidateCanonFact(candidate); err != nil {
				return zero, fmt.Errorf("roleplay canon fact inventory candidate %d: %w", index, err)
			}
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
	if inventory.Candidates == nil || len(inventory.Candidates) > MaxRoleplayCanonFactsPerTurn {
		return fmt.Errorf("roleplay canon fact inventory must contain 0..%d candidates", MaxRoleplayCanonFactsPerTurn)
	}
	for index, candidate := range inventory.Candidates {
		if strings.ContainsAny(candidate, "\r\n") {
			return fmt.Errorf("roleplay canon fact inventory candidate %d must be one line", index)
		}
		if err := roleplay.ValidateCanonFact(candidate); err != nil {
			return fmt.Errorf("roleplay canon fact inventory candidate %d: %w", index, err)
		}
	}
	raw := RoleplayNoCanonFactCandidates
	if len(inventory.Candidates) > 0 {
		raw = strings.Join(inventory.Candidates, "\n")
	}
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
	modelContext, err := projectObjectiveContextForModel(input.Context)
	if err != nil {
		return "", err
	}
	var authority strings.Builder
	fmt.Fprintf(&authority, "SOURCE KIND:\n%s\n", input.Source.Kind)
	fmt.Fprintf(&authority, "ATTRIBUTED PERSONA:\n%s\n", input.Source.AttributedPersonaName)
	if input.Source.Kind == RoleplayCanonSourceUserContribution {
		fmt.Fprintf(&authority, "PERSONA KIND:\n%s\n", input.Source.PersonaKind)
		fmt.Fprintf(&authority, "CONTRIBUTION KIND:\n%s\n", input.Source.ContributionKind)
	}
	fmt.Fprintf(&authority, "EXACT CONTRIBUTION:\n%s\n", input.Source.ExactContribution)
	if input.AntecedentUserTurn != nil {
		antecedent := input.AntecedentUserTurn
		fmt.Fprintf(&authority, "ANTECEDENT PERSONA KIND:\n%s\n", antecedent.PersonaKind)
		fmt.Fprintf(&authority, "ANTECEDENT PERSONA NAME:\n%s\n", antecedent.PersonaName)
		fmt.Fprintf(&authority, "ANTECEDENT CONTRIBUTION KIND:\n%s\n", antecedent.ContributionKind)
		fmt.Fprintf(&authority, "ANTECEDENT CONTRIBUTION:\n%s\n", antecedent.ContributionContext)
	}
	if len(modelContext.Capsules) == 0 {
		authority.WriteString("ESTABLISHED CONTEXT:\n(none)\n")
	} else {
		for index, capsule := range modelContext.Capsules {
			fmt.Fprintf(&authority, "ESTABLISHED CONTEXT %d:\n%s\n", index+1, capsule)
		}
	}
	if authority.Len() > maxPortablePayloadBytes {
		return "", fmt.Errorf("roleplay canon fact authority exceeds %d bytes", maxPortablePayloadBytes)
	}
	return strings.TrimSuffix(authority.String(), "\n"), nil
}

func roleplayCanonSemanticAuthoritySHA256(value any) (string, error) {
	authority, err := exactjson.Canonical(value)
	if err != nil {
		return "", fmt.Errorf("encode roleplay canon semantic authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
