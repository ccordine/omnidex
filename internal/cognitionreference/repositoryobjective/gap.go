package repositoryobjective

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

type subjectGapBinding struct {
	gap        cognitionreference.SemanticGap
	candidates map[cognitionreference.CandidateID]subjectCandidate
}

func buildSubjectGap(objective Objective, analysisID string, candidates []subjectCandidate) (subjectGapBinding, error) {
	if objective.Subject.Kind != LookupName || len(candidates) < 2 {
		return subjectGapBinding{}, fmt.Errorf("%w: subject gap requires ambiguous unqualified lookup", ErrSemanticResolution)
	}
	if objective.Question == "" {
		return subjectGapBinding{}, fmt.Errorf("%w: ambiguous subject requires one exact question", ErrSemanticResolution)
	}
	evidence := make([]cognitionreference.SemanticEvidence, len(candidates))
	choices := make([]cognitionreference.SemanticCandidate, len(candidates))
	mapping := make(map[cognitionreference.CandidateID]subjectCandidate, len(candidates))
	for index, candidate := range candidates {
		if len(candidate.span.Content) > maxGapDeclarationBytes {
			return subjectGapBinding{}, fmt.Errorf(
				"%w: candidate %q declaration exceeds %d-byte semantic evidence bound",
				ErrSemanticResolution, candidate.symbol.ID, maxGapDeclarationBytes,
			)
		}
		evidenceID := cognitionreference.EvidenceID(fmt.Sprintf("E%02d", index+1))
		candidateID := cognitionreference.CandidateID(fmt.Sprintf("C%02d", index+1))
		evidence[index] = cognitionreference.SemanticEvidence{ID: evidenceID, Content: candidate.span.Content}
		choices[index] = cognitionreference.SemanticCandidate{
			ID: candidateID, Summary: candidate.symbol.Kind + " " + candidate.symbol.Signature,
			EvidenceIDs: []cognitionreference.EvidenceID{evidenceID},
		}
		mapping[candidateID] = candidate
	}
	gap := cognitionreference.SemanticGap{
		ID:   gapIdentity(objective, analysisID, candidates),
		Kind: cognitionreference.GapCandidateSelection, ObjectiveID: objective.ID,
		Question: objective.Question, Evidence: evidence, Candidates: choices,
	}
	if err := gap.Validate(); err != nil {
		return subjectGapBinding{}, fmt.Errorf("%w: %v", ErrSemanticResolution, err)
	}
	return subjectGapBinding{
		gap: gap, candidates: mapping,
	}, nil
}

func gapIdentity(objective Objective, analysisID string, candidates []subjectCandidate) cognitionreference.GapID {
	hash := sha256.New()
	writeGapIdentity(
		hash, string(objective.ID), analysisID, objective.Question,
		string(objective.Subject.Kind), objective.Subject.Value,
	)
	for _, predicate := range objective.Acceptance {
		writeGapIdentity(hash, string(predicate))
	}
	for _, candidate := range candidates {
		writeGapIdentity(hash, candidate.symbol.ID, candidate.evidence.DeclarationSHA256)
	}
	return cognitionreference.GapID("repository-subject-" + hex.EncodeToString(hash.Sum(nil)))
}

type identityWriter interface{ Write([]byte) (int, error) }

func writeGapIdentity(writer identityWriter, values ...string) {
	for _, value := range values {
		_, _ = writer.Write([]byte(value))
		_, _ = writer.Write([]byte{0})
	}
}

func resolveSubject(
	ctx context.Context,
	objective Objective,
	analysisID string,
	candidates []subjectCandidate,
	selector cognitionreference.Selector,
	result *Result,
) (SubjectFact, subjectCandidate, error) {
	if len(candidates) == 1 {
		if objective.Question != "" {
			return SubjectFact{}, subjectCandidate{}, fmt.Errorf(
				"%w: deterministic subject resolution forbids a semantic question",
				ErrInvalidObjective,
			)
		}
		fact := makeSubjectFact(objective, analysisID, candidates[0], SubjectAuthorityDeterministic, "", "")
		return fact, candidates[0], nil
	}
	binding, err := buildSubjectGap(objective, analysisID, candidates)
	if err != nil {
		return SubjectFact{}, subjectCandidate{}, err
	}
	if selector == nil {
		return SubjectFact{}, subjectCandidate{}, fmt.Errorf("%w: ambiguous subject has no selector", ErrSemanticResolution)
	}
	counted := &countingSelector{delegate: selector}
	selected, err := cognitionreference.SelectCandidate(ctx, counted, binding.gap)
	result.SelectorCalls += counted.calls
	result.InferenceCalls += counted.calls
	if err != nil {
		return SubjectFact{}, subjectCandidate{}, fmt.Errorf("%w: %w", ErrSemanticResolution, err)
	}
	candidate, exists := binding.candidates[selected]
	if !exists {
		return SubjectFact{}, subjectCandidate{}, fmt.Errorf("%w: selected candidate has no code-held mapping", ErrSemanticResolution)
	}
	fact := makeSubjectFact(objective, analysisID, candidate, SubjectAuthoritySemantic, binding.gap.ID, selected)
	return fact, candidate, nil
}

type countingSelector struct {
	delegate cognitionreference.Selector
	calls    int
}

func (selector *countingSelector) Select(
	ctx context.Context,
	gap cognitionreference.SemanticGap,
) (cognitionreference.CandidateID, error) {
	selector.calls++
	return selector.delegate.Select(ctx, gap)
}

func makeSubjectFact(
	objective Objective,
	analysisID string,
	candidate subjectCandidate,
	authority SubjectAuthority,
	gapID cognitionreference.GapID,
	candidateID cognitionreference.CandidateID,
) SubjectFact {
	return SubjectFact{
		ObjectiveID: objective.ID, Acceptance: cloneAcceptance(objective.Acceptance),
		AnalysisID: analysisID, Authority: authority, GapID: gapID, CandidateID: candidateID,
		Symbol: candidate.evidence,
	}
}
