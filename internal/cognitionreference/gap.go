package cognitionreference

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type GapID string
type GapKind string
type CandidateID string
type EvidenceID string

const GapCandidateSelection GapKind = "candidate_selection"

const (
	maxGapQuestionBytes      = 2 * 1024
	maxGapEvidence           = 16
	maxGapEvidenceBytes      = 2 * 1024
	minGapCandidates         = 2
	maxGapCandidates         = 8
	maxCandidateSummaryBytes = 1024
	maxCandidateEvidence     = maxGapEvidence
)

var (
	ErrInvalidSemanticGap = errors.New("invalid cognition reference semantic gap")
	ErrInvalidSelection   = errors.New("invalid cognition reference candidate selection")
)

// SemanticEvidence is bounded semantic input, not authority for invoking
// machinery. Its opaque identity is resolved only by code outside the model
// boundary.
type SemanticEvidence struct {
	ID      EvidenceID `json:"id"`
	Content string     `json:"content"`
}

// SemanticCandidate exposes only an opaque choice and its bounded meaning.
// It cannot encode an operation, argument, tool, or state transition.
type SemanticCandidate struct {
	ID          CandidateID  `json:"id"`
	Summary     string       `json:"summary"`
	EvidenceIDs []EvidenceID `json:"evidence_ids"`
}

// SemanticGap names one irreducible question for one exact code-held
// objective. Candidates and evidence are canonical, immutable-by-copy inputs.
type SemanticGap struct {
	ID          GapID               `json:"id"`
	Kind        GapKind             `json:"kind"`
	ObjectiveID ObjectiveID         `json:"objective_id"`
	Question    string              `json:"question"`
	Evidence    []SemanticEvidence  `json:"evidence"`
	Candidates  []SemanticCandidate `json:"candidates"`
}

// Selector crosses one semantic gap. The only accepted result is one opaque
// candidate identity; it cannot request or execute deterministic machinery.
type Selector interface {
	Select(context.Context, SemanticGap) (CandidateID, error)
}

func (gap SemanticGap) Validate() error {
	if err := validIdentity(string(gap.ID)); err != nil {
		return fmt.Errorf("%w: gap ID: %v", ErrInvalidSemanticGap, err)
	}
	if gap.Kind != GapCandidateSelection {
		return fmt.Errorf("%w: unregistered gap kind %q", ErrInvalidSemanticGap, gap.Kind)
	}
	if err := validIdentity(string(gap.ObjectiveID)); err != nil {
		return fmt.Errorf("%w: objective ID: %v", ErrInvalidSemanticGap, err)
	}
	if err := validateGapText(gap.Question, maxGapQuestionBytes, "question"); err != nil {
		return err
	}
	evidence, err := gap.validateEvidence()
	if err != nil {
		return err
	}
	if len(gap.Candidates) < minGapCandidates || len(gap.Candidates) > maxGapCandidates {
		return fmt.Errorf(
			"%w: candidate count must be between %d and %d",
			ErrInvalidSemanticGap, minGapCandidates, maxGapCandidates,
		)
	}
	usedEvidence := make(map[EvidenceID]struct{}, len(evidence))
	previous := CandidateID("")
	for index, candidate := range gap.Candidates {
		if err := validIdentity(string(candidate.ID)); err != nil {
			return fmt.Errorf("%w: candidate %d ID: %v", ErrInvalidSemanticGap, index, err)
		}
		if index > 0 && candidate.ID <= previous {
			return fmt.Errorf("%w: candidates must have unique ascending IDs", ErrInvalidSemanticGap)
		}
		previous = candidate.ID
		if err := validateGapText(candidate.Summary, maxCandidateSummaryBytes, "candidate summary"); err != nil {
			return err
		}
		if len(candidate.EvidenceIDs) == 0 || len(candidate.EvidenceIDs) > maxCandidateEvidence {
			return fmt.Errorf("%w: candidate %q has invalid evidence cardinality", ErrInvalidSemanticGap, candidate.ID)
		}
		previousEvidence := EvidenceID("")
		for evidenceIndex, id := range candidate.EvidenceIDs {
			if _, exists := evidence[id]; !exists {
				return fmt.Errorf("%w: candidate %q cites unknown evidence %q", ErrInvalidSemanticGap, candidate.ID, id)
			}
			if evidenceIndex > 0 && id <= previousEvidence {
				return fmt.Errorf("%w: candidate %q evidence IDs must be unique and ascending", ErrInvalidSemanticGap, candidate.ID)
			}
			previousEvidence = id
			usedEvidence[id] = struct{}{}
		}
	}
	if len(usedEvidence) != len(evidence) {
		return fmt.Errorf("%w: unreferenced evidence is not a minimal gap input", ErrInvalidSemanticGap)
	}
	return nil
}

func (gap SemanticGap) validateEvidence() (map[EvidenceID]struct{}, error) {
	if len(gap.Evidence) == 0 || len(gap.Evidence) > maxGapEvidence {
		return nil, fmt.Errorf("%w: evidence count must be between 1 and %d", ErrInvalidSemanticGap, maxGapEvidence)
	}
	registered := make(map[EvidenceID]struct{}, len(gap.Evidence))
	previous := EvidenceID("")
	for index, evidence := range gap.Evidence {
		if err := validIdentity(string(evidence.ID)); err != nil {
			return nil, fmt.Errorf("%w: evidence %d ID: %v", ErrInvalidSemanticGap, index, err)
		}
		if index > 0 && evidence.ID <= previous {
			return nil, fmt.Errorf("%w: evidence must have unique ascending IDs", ErrInvalidSemanticGap)
		}
		previous = evidence.ID
		if err := validateGapText(evidence.Content, maxGapEvidenceBytes, "evidence content"); err != nil {
			return nil, err
		}
		registered[evidence.ID] = struct{}{}
	}
	return registered, nil
}

func validateGapText(value string, maxBytes int, label string) error {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) ||
		strings.ContainsRune(value, 0) || strings.TrimSpace(value) != value {
		return fmt.Errorf("%w: %s must be nonempty bounded exact UTF-8", ErrInvalidSemanticGap, label)
	}
	return nil
}

func (gap SemanticGap) Clone() SemanticGap {
	gap.Evidence = append([]SemanticEvidence{}, gap.Evidence...)
	gap.Candidates = append([]SemanticCandidate{}, gap.Candidates...)
	for index := range gap.Candidates {
		gap.Candidates[index].EvidenceIDs = append([]EvidenceID{}, gap.Candidates[index].EvidenceIDs...)
	}
	return gap
}

func (gap SemanticGap) ValidateSelection(selected CandidateID) error {
	if err := gap.Validate(); err != nil {
		return err
	}
	if err := validIdentity(string(selected)); err != nil {
		return fmt.Errorf("%w: candidate ID: %v", ErrInvalidSelection, err)
	}
	for _, candidate := range gap.Candidates {
		if candidate.ID == selected {
			return nil
		}
	}
	return fmt.Errorf("%w: candidate %q is outside gap %q", ErrInvalidSelection, selected, gap.ID)
}

func SelectCandidate(ctx context.Context, selector Selector, gap SemanticGap) (CandidateID, error) {
	if ctx == nil {
		return "", fmt.Errorf("%w: context is nil", ErrInvalidSelection)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if selector == nil {
		return "", fmt.Errorf("%w: selector is nil", ErrInvalidSelection)
	}
	gap = gap.Clone()
	if err := gap.Validate(); err != nil {
		return "", err
	}
	selected, err := selector.Select(ctx, gap.Clone())
	if contextErr := ctx.Err(); contextErr != nil {
		return "", contextErr
	}
	if err != nil {
		return "", fmt.Errorf("semantic gap %q selector: %w", gap.ID, err)
	}
	if err := gap.ValidateSelection(selected); err != nil {
		return "", err
	}
	return selected, nil
}
