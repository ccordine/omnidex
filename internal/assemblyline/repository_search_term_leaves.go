package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkRepositorySearchAnchorCoverage WorkKind = "repository_search_anchor_coverage"
	WorkRepositorySearchAnchor         WorkKind = "repository_search_anchor"

	MaxRepositorySearchAnchorLeaves = maxRepositorySearchAnchors
	RepositoryAnchorRemains         = "ANCHOR_REMAINS"
	RepositoryNoUncoveredAnchor     = "NO_UNCOVERED_ANCHOR"
)

// RepositorySearchAnchorLeafInput carries one unresolved concept and the
// code-retained anchors already accepted for it.
type RepositorySearchAnchorLeafInput struct {
	UnresolvedConcept string   `json:"unresolved_concept"`
	AcceptedAnchors   []string `json:"accepted_anchors"`
}

func NewRepositorySearchAnchorCoverageJob(
	input RepositorySearchAnchorLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkRepositorySearchAnchorCoverage, input, input.validate,
	)
}

func NewRepositorySearchAnchorJob(
	input RepositorySearchAnchorLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkRepositorySearchAnchor, input, input.validate,
	)
}

func (input RepositorySearchAnchorLeafInput) validate() error {
	base := RepositorySearchTermInput{UnresolvedConcept: input.UnresolvedConcept}
	if err := base.validate(); err != nil {
		return err
	}
	if input.AcceptedAnchors == nil {
		return fmt.Errorf("repository search anchor leaf requires a non-nil accepted set")
	}
	if len(input.AcceptedAnchors) > MaxRepositorySearchAnchorLeaves {
		return fmt.Errorf(
			"repository search anchor leaf exceeds %d accepted anchors",
			MaxRepositorySearchAnchorLeaves,
		)
	}
	seen := make(map[string]struct{}, len(input.AcceptedAnchors))
	for index, anchor := range input.AcceptedAnchors {
		if err := validateRepositorySearchText(
			fmt.Sprintf("accepted anchor %d", index), anchor,
			maxRepositorySearchTermBytes,
		); err != nil {
			return err
		}
		if strings.ContainsAny(anchor, "\r\n") {
			return fmt.Errorf("repository search accepted anchor %d must be one line", index)
		}
		if err := ValidatePathFreeModelContext("repository search accepted anchor", anchor); err != nil {
			return err
		}
		if _, duplicate := seen[anchor]; duplicate {
			return fmt.Errorf("repository search accepted anchor %d is duplicated", index)
		}
		seen[anchor] = struct{}{}
	}
	return nil
}

func BuildRepositorySearchAnchorCoveragePrompt(
	input RepositorySearchAnchorLeafInput,
) (string, error) {
	authority, err := renderRepositorySearchAnchorAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic coverage relation: would one additional materially distinct lexical anchor improve deterministic repository lookup for the unresolved concept?",
		"An anchor is a likely declaration name, symbol fragment, domain noun, or short phrase that could occur in an existing declaration name or signature. It must not be a path, operation, command, implementation proposal, or search plan.",
		"Return ANCHOR_REMAINS when one useful anchor remains uncovered. Return NO_UNCOVERED_ANCHOR when the accepted anchors are sufficient.",
		"Return exactly that registered raw value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"REPOSITORY_SEARCH_ANCHOR_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeRepositorySearchAnchorCoverageLeaf(
	input RepositorySearchAnchorLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"repository search anchor coverage", raw, 32, false,
	)
	if err != nil {
		return "", err
	}
	switch leaf {
	case RepositoryAnchorRemains, RepositoryNoUncoveredAnchor:
		return leaf, nil
	default:
		return "", fmt.Errorf(
			"repository search anchor coverage value %q is not registered", leaf,
		)
	}
}

func BuildRepositorySearchAnchorPrompt(
	input RepositorySearchAnchorLeafInput,
) (string, error) {
	authority, err := renderRepositorySearchAnchorAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return exactly one materially distinct lexical anchor for locating an existing declaration that could answer the unresolved concept.",
		"Choose the most discriminating useful anchor not semantically covered by the accepted anchors. Return a likely declaration name, symbol fragment, domain noun, or short phrase, never a path, operation, command, implementation proposal, or search plan.",
		"Return only the anchor as one raw line. Do not return JSON, quotes, a label, Markdown, or commentary.",
		"REPOSITORY_SEARCH_ANCHOR_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeRepositorySearchAnchorLeaf(
	input RepositorySearchAnchorLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"repository search anchor", raw, maxRepositorySearchTermBytes, false,
	)
	if err != nil {
		return "", err
	}
	if err := validateRepositorySearchText(
		"anchor", leaf, maxRepositorySearchTermBytes,
	); err != nil {
		return "", err
	}
	if err := ValidatePathFreeModelContext("repository search anchor", leaf); err != nil {
		return "", err
	}
	for _, accepted := range input.AcceptedAnchors {
		if leaf == accepted {
			return "", fmt.Errorf("repository search anchor duplicates an accepted anchor")
		}
	}
	return leaf, nil
}

func AssembleRepositorySearchTermDecision(
	input RepositorySearchTermInput,
	anchors []string,
) (RepositorySearchTermDecision, error) {
	decision := RepositorySearchTermDecision{
		Schema:  RepositorySearchTermSchemaV2,
		Anchors: append([]string{}, anchors...),
	}
	if err := decision.ValidateFor(input); err != nil {
		return RepositorySearchTermDecision{}, err
	}
	return decision, nil
}

func renderRepositorySearchAnchorAuthority(
	input RepositorySearchAnchorLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var authority strings.Builder
	fmt.Fprintf(&authority, "UNRESOLVED CONCEPT:\n%s\n", input.UnresolvedConcept)
	if len(input.AcceptedAnchors) == 0 {
		authority.WriteString("ACCEPTED ANCHORS:\n(none)\n")
	} else {
		for index, anchor := range input.AcceptedAnchors {
			fmt.Fprintf(&authority, "ACCEPTED ANCHOR %d:\n%s\n", index+1, anchor)
		}
	}
	return strings.TrimSuffix(authority.String(), "\n"), nil
}
