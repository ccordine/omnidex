package webresearch

import (
	"fmt"

	"github.com/gryph/omnidex/internal/websearch"
)

// EvidenceFromDocuments converts already fetched documents into immutable
// citation identities without invoking any semantic station.
func EvidenceFromDocuments(documents []websearch.Document) ([]Evidence, error) {
	if len(documents) == 0 {
		return nil, fmt.Errorf("%w: no fetched documents exist", ErrEvidenceUnavailable)
	}
	for index, document := range documents {
		if err := websearch.ValidateDocument(document); err != nil {
			return nil, fmt.Errorf("%w: document %d: %v", ErrInvalidAcquisition, index, err)
		}
	}
	evidence := evidenceFromDocuments(documents)
	if err := validateEvidence(evidence); err != nil {
		return nil, err
	}
	return cloneEvidence(evidence), nil
}

// BuildGroundedCompletionArtifact deterministically binds paragraph evidence
// IDs to source numbers and URLs. It performs no inference or review.
func BuildGroundedCompletionArtifact(
	paragraphs []GroundedParagraph,
	evidence []Evidence,
	maxParagraphs, maxParagraphBytes int,
) (Artifact, error) {
	if maxParagraphs < 1 || maxParagraphs > maxPortableSynthesisParagraphs ||
		maxParagraphBytes < 1 || maxParagraphBytes > maxPortableSynthesisParagraphBytes {
		return Artifact{}, fmt.Errorf("%w: grounded artifact bounds are invalid", ErrInvalidConfiguration)
	}
	if err := validateEvidence(evidence); err != nil {
		return Artifact{}, err
	}
	projected := make([]ProjectedEvidence, len(evidence))
	for index, item := range evidence {
		if err := validateAcquiredArtifactEvidence(item); err != nil {
			return Artifact{}, err
		}
		projected[index] = ProjectedEvidence{
			EvidenceID: item.ID, CandidateID: item.CandidateID,
			Title: item.Title, Snippet: item.Snippet, Content: item.Content,
		}
	}
	artifact, err := buildArtifact(
		GroundedSynthesisDecision{Paragraphs: cloneParagraphs(paragraphs)},
		projected, cloneEvidence(evidence), Config{
			MaxSynthesisParagraphs:     maxParagraphs,
			MaxSynthesisParagraphBytes: maxParagraphBytes,
		},
	)
	if err != nil {
		return Artifact{}, err
	}
	if err := ValidateCompletionArtifact(artifact, evidence); err != nil {
		return Artifact{}, err
	}
	return cloneArtifact(artifact), nil
}
