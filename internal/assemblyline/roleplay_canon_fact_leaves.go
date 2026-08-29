package assemblyline

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	WorkRoleplayCanonFactCoverage WorkKind = "roleplay_canon_fact_coverage"
	WorkRoleplayCanonFact         WorkKind = "roleplay_canon_fact"

	RoleplayCanonFactRemains     = "CANON_FACT_REMAINS"
	RoleplayNoUncoveredCanonFact = "NO_UNCOVERED_CANON_FACT"
)

// RoleplayCanonFactLeafInput retains one exact contribution and the canon
// facts code has already accepted from that contribution.
type RoleplayCanonFactLeafInput struct {
	Source             RoleplayCanonSource      `json:"source"`
	AntecedentUserTurn *RoleplayCanonAntecedent `json:"antecedent_user_turn,omitempty"`
	Context            ObjectiveContext         `json:"context"`
	AcceptedFacts      []string                 `json:"accepted_facts"`
}

func NewRoleplayCanonFactCoverageJob(
	input RoleplayCanonFactLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkRoleplayCanonFactCoverage, input, input.validate,
	)
}

func NewRoleplayCanonFactJob(
	input RoleplayCanonFactLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkRoleplayCanonFact, input, input.validate,
	)
}

func (input RoleplayCanonFactLeafInput) extractionInput() RoleplayCanonExtractionInput {
	return RoleplayCanonExtractionInput{
		Source: input.Source, AntecedentUserTurn: input.AntecedentUserTurn,
		Context: input.Context,
	}
}

func (input RoleplayCanonFactLeafInput) validate() error {
	base := input.extractionInput()
	if err := base.validate(); err != nil {
		return err
	}
	if input.AcceptedFacts == nil {
		return fmt.Errorf("roleplay canon fact leaf requires a non-nil accepted set")
	}
	return (RoleplayCanonExtractionDecision{
		Schema: RoleplayCanonExtractionSchemaV1,
		Facts:  append([]string{}, input.AcceptedFacts...),
	}).ValidateFor(base)
}

func BuildRoleplayCanonFactCoveragePrompt(
	input RoleplayCanonFactLeafInput,
) (string, error) {
	authority, err := renderRoleplayCanonFactAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic coverage relation: does the exact current contribution establish one durable fictional fact that is not semantically covered by the accepted current-contribution facts?",
		"Treat only the exact contribution as the fact source. Established context is reference material, and an antecedent user turn only resolves references in an assistant contribution. Questions, requests, and directions are not fictional events.",
		"Exclude implications, restatements of established context, inferred character visibility, decorative sensory detail, and real-world claims. Attribute first-person statements, actions, possessions, and knowledge only to " + strconv.Quote(input.Source.AttributedPersonaName) + ".",
		"Return CANON_FACT_REMAINS when one qualifying fact remains. Return NO_UNCOVERED_CANON_FACT when none remains.",
		"Return exactly that registered raw value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"ROLEPLAY_CANON_FACT_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeRoleplayCanonFactCoverageLeaf(
	input RoleplayCanonFactLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"roleplay canon fact coverage", raw, 32, false,
	)
	if err != nil {
		return "", err
	}
	switch leaf {
	case RoleplayCanonFactRemains, RoleplayNoUncoveredCanonFact:
		return leaf, nil
	default:
		return "", fmt.Errorf(
			"roleplay canon fact coverage value %q is not registered", leaf,
		)
	}
}

func BuildRoleplayCanonFactPrompt(
	input RoleplayCanonFactLeafInput,
) (string, error) {
	authority, err := renderRoleplayCanonFactAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return exactly one durable fictional fact established by the exact current contribution and not semantically covered by the accepted current-contribution facts.",
		"Treat established context only as reference material and an antecedent user turn only as referent authority. Exclude questions, requests, directions, implications, restatements, inferred visibility, decorative sensory detail, and real-world claims.",
		"Write one concise standalone fact. Attribute every first-person statement, action, possession, or item of knowledge only to " + strconv.Quote(input.Source.AttributedPersonaName) + ".",
		"Return only the fact as one raw line. Do not return JSON, quotes, a label, Markdown, or commentary.",
		"ROLEPLAY_CANON_FACT_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeRoleplayCanonFactLeaf(
	input RoleplayCanonFactLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"roleplay canon fact", raw, roleplay.MaxCanonEventBytes, false,
	)
	if err != nil {
		return "", err
	}
	if err := roleplay.ValidateCanonFact(leaf); err != nil {
		return "", err
	}
	for _, accepted := range input.AcceptedFacts {
		if leaf == accepted {
			return "", fmt.Errorf("roleplay canon fact duplicates an accepted fact")
		}
	}
	return leaf, nil
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

func renderRoleplayCanonFactAuthority(
	input RoleplayCanonFactLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	modelContext, err := projectObjectiveContextForModel(input.Context)
	if err != nil {
		return "", err
	}
	var authority strings.Builder
	fmt.Fprintf(&authority, "SOURCE KIND:\n%s\n", input.Source.Kind)
	fmt.Fprintf(
		&authority, "ATTRIBUTED PERSONA:\n%s\n", input.Source.AttributedPersonaName,
	)
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
	if len(input.AcceptedFacts) == 0 {
		authority.WriteString("ACCEPTED CURRENT-CONTRIBUTION FACTS:\n(none)\n")
	} else {
		for index, fact := range input.AcceptedFacts {
			fmt.Fprintf(
				&authority, "ACCEPTED CURRENT-CONTRIBUTION FACT %d:\n%s\n",
				index+1, fact,
			)
		}
	}
	if authority.Len() > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"roleplay canon fact authority exceeds %d bytes", maxPortablePayloadBytes,
		)
	}
	return strings.TrimSuffix(authority.String(), "\n"), nil
}
