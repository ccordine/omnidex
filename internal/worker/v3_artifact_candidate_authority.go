package worker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

type artifactCandidateAuthority struct {
	input     assemblyline.ArtifactCandidateEvidence
	directive assemblyline.ArtifactDirective
	file      repositoryfacts.File
}

func resolveAmbiguousArtifactDeletion(
	runtime typedWorkerRuntime,
	modelName string,
	featureQuotes []string,
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) ([]assemblyline.ArtifactDirective, error) {
	candidateCount := 0
	for _, directive := range directives {
		switch directive.Disposition {
		case assemblyline.ArtifactAbsenceCandidate:
			candidateCount++
		case assemblyline.ArtifactForbid:
			return nil, fmt.Errorf("resolved and ambiguous artifact absence authority cannot be mixed")
		}
	}
	if candidateCount == 0 {
		return append([]assemblyline.ArtifactDirective(nil), directives...), nil
	}
	if candidateCount < 2 || candidateCount > 8 {
		return nil, fmt.Errorf("ambiguous artifact absence requires a safe finite set of 2-8 candidates")
	}
	quote, err := exactArtifactCandidateRequirementQuote(directives, featureQuotes)
	if err != nil {
		return nil, err
	}
	authorities, err := buildArtifactCandidateAuthorities(
		runtime.Context, quote, directives, identities, snapshot, analysis,
	)
	if err != nil {
		return nil, err
	}
	input := assemblyline.ArtifactCandidateSelectionInput{
		RequirementQuote: quote,
		Candidates:       make([]assemblyline.ArtifactCandidateEvidence, len(authorities)),
	}
	for index := range authorities {
		input.Candidates[index] = authorities[index].input
	}
	decision, err := selectArtifactCandidate(runtime, modelName, input, identities)
	if err != nil {
		return nil, err
	}
	if decision.CandidateID == assemblyline.ArtifactCandidateSelectionNone {
		return nil, fmt.Errorf("artifact candidate selection returned NONE; exact deletion target is unsupported")
	}
	selectedToken := ""
	for _, authority := range authorities {
		if authority.input.CandidateID == decision.CandidateID {
			selectedToken = authority.directive.Token
			break
		}
	}
	if selectedToken == "" {
		return nil, fmt.Errorf("artifact candidate selection returned unavailable authority")
	}
	resolved := append([]assemblyline.ArtifactDirective(nil), directives...)
	for index := range resolved {
		if resolved[index].Disposition != assemblyline.ArtifactAbsenceCandidate {
			continue
		}
		resolved[index].Disposition = assemblyline.ArtifactReference
		if resolved[index].Token == selectedToken {
			resolved[index].Disposition = assemblyline.ArtifactForbid
		}
	}
	return resolved, nil
}

func exactArtifactCandidateRequirementQuote(
	directives []assemblyline.ArtifactDirective,
	featureQuotes []string,
) (string, error) {
	quote := ""
	for _, directive := range directives {
		if directive.Disposition != assemblyline.ArtifactAbsenceCandidate {
			continue
		}
		matched := ""
		for _, candidate := range featureQuotes {
			if !containsExactArtifactToken(candidate, directive.Token) {
				continue
			}
			if matched != "" {
				return "", fmt.Errorf("artifact absence candidate %q belongs to multiple exact requirements", directive.Token)
			}
			matched = candidate
		}
		if matched == "" {
			return "", fmt.Errorf("artifact absence candidate %q has no exact requirement binding", directive.Token)
		}
		if quote != "" && quote != matched {
			return "", fmt.Errorf("multiple ambiguous artifact absence groups are unsupported")
		}
		quote = matched
	}
	if quote == "" {
		return "", fmt.Errorf("ambiguous artifact absence has no exact requirement")
	}
	return quote, nil
}

func buildArtifactCandidateAuthorities(
	ctx context.Context,
	requirementQuote string,
	directives []assemblyline.ArtifactDirective,
	identities []assemblyline.ArtifactIdentity,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) ([]artifactCandidateAuthority, error) {
	identityByToken := make(map[string]string, len(identities))
	for _, identity := range identities {
		if _, duplicate := identityByToken[identity.Token]; duplicate {
			return nil, fmt.Errorf("artifact candidate identity %q is duplicated", identity.Token)
		}
		identityByToken[identity.Token] = identity.Value
	}
	fileByPath := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		fileByPath[file.Path] = file
	}
	candidates := make([]artifactCandidateAuthority, 0)
	for _, directive := range directives {
		if directive.Disposition != assemblyline.ArtifactAbsenceCandidate {
			continue
		}
		value, exists := identityByToken[directive.Token]
		if !exists || !containsExactArtifactToken(requirementQuote, directive.Token) {
			return nil, fmt.Errorf("artifact absence candidate %q lacks exact code-held identity", directive.Token)
		}
		file, exists := fileByPath[value]
		if !exists || file.Path != value {
			return nil, fmt.Errorf("artifact absence candidate %q is not an exact indexed member", directive.Token)
		}
		if err := changeapply.ValidateDeletionCandidate(
			ctx, snapshot, analysis, file.ID,
		); err != nil {
			return nil, fmt.Errorf("artifact absence candidate %q is ineligible: %w", directive.Token, err)
		}
		declarations, err := artifactCandidateDeclarations(analysis, file)
		if err != nil {
			return nil, fmt.Errorf("artifact absence candidate %q: %w", directive.Token, err)
		}
		candidates = append(candidates, artifactCandidateAuthority{
			directive: directive, file: file,
			input: assemblyline.ArtifactCandidateEvidence{
				Declarations: declarations,
			},
		})
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].file.Path == candidates[right].file.Path {
			return candidates[left].file.ID < candidates[right].file.ID
		}
		return candidates[left].file.Path < candidates[right].file.Path
	})
	for index := range candidates {
		candidates[index].input.CandidateID = fmt.Sprintf("ARTIFACT_CANDIDATE_%d", index+1)
	}
	return candidates, nil
}

func artifactCandidateDeclarations(
	analysis repositoryfacts.Analysis,
	file repositoryfacts.File,
) ([]string, error) {
	declarations := make([]string, 0)
	for _, symbol := range analysis.Symbols {
		if symbol.FileID != file.ID {
			continue
		}
		value := strings.TrimSpace(symbol.Kind + " " + symbol.Name + ": " + symbol.Signature)
		if value == "" || strings.ContainsAny(value, "\x00\r\n") || len(value) > 256 {
			return nil, fmt.Errorf("candidate contains unsupported declaration evidence")
		}
		declarations = append(declarations, value)
	}
	sort.Strings(declarations)
	if len(declarations) < 1 || len(declarations) > 4 {
		return nil, fmt.Errorf("candidate requires 1-4 bounded indexed declarations")
	}
	return declarations, nil
}
