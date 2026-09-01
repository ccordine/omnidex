package webresearch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/websearch"
)

var modelCitationSyntax = regexp.MustCompile(`(?i)https?://|\[[0-9]+\]`)

const maxEvidenceIDsPerParagraph = 4

// ValidateCompletionArtifact proves that a claimed completion is exactly the
// code-rendered projection of its acquired evidence. A matching self-supplied
// hash is not sufficient authority.
func ValidateCompletionArtifact(artifact Artifact, allEvidence []Evidence) error {
	if len(artifact.Paragraphs) < 1 || len(artifact.Paragraphs) > maxPortableSynthesisParagraphs ||
		len(artifact.Sources) < 1 || len(artifact.Sources) > maxPortableEvidence ||
		len(allEvidence) < 1 || len(allEvidence) > maxPortableEvidence {
		return fmt.Errorf("%w: completion artifact exceeds hard cardinality bounds", ErrInvalidSynthesis)
	}
	if err := validateEvidence(allEvidence); err != nil {
		return err
	}
	byID := make(map[EvidenceID]Evidence, len(allEvidence))
	for _, item := range allEvidence {
		if err := validateAcquiredArtifactEvidence(item); err != nil {
			return err
		}
		byID[item.ID] = item
	}
	cited := make(map[EvidenceID]struct{})
	for index, paragraph := range artifact.Paragraphs {
		if paragraph.Text == "" || paragraph.Text != strings.TrimSpace(paragraph.Text) ||
			len(paragraph.Text) > maxPortableSynthesisParagraphBytes || !utf8.ValidString(paragraph.Text) ||
			strings.ContainsRune(paragraph.Text, '\x00') || modelCitationSyntax.MatchString(paragraph.Text) ||
			len(paragraph.EvidenceIDs) < 1 || len(paragraph.EvidenceIDs) > maxEvidenceIDsPerParagraph {
			return fmt.Errorf("%w: completion paragraph %d is invalid", ErrInvalidSynthesis, index)
		}
		seen := make(map[EvidenceID]struct{}, len(paragraph.EvidenceIDs))
		for _, id := range paragraph.EvidenceIDs {
			if _, exists := byID[id]; !exists {
				return fmt.Errorf("%w: completion paragraph %d cites unknown evidence %q", ErrInvalidSynthesis, index, id)
			}
			if _, duplicate := seen[id]; duplicate {
				return fmt.Errorf("%w: completion paragraph %d duplicates evidence %q", ErrInvalidSynthesis, index, id)
			}
			seen[id] = struct{}{}
			cited[id] = struct{}{}
		}
	}
	expectedSources := make([]CitationSource, 0, len(cited))
	numbers := make(map[EvidenceID]int, len(cited))
	for _, item := range allEvidence {
		if _, used := cited[item.ID]; !used {
			continue
		}
		number := len(expectedSources) + 1
		numbers[item.ID] = number
		expectedSources = append(expectedSources, CitationSource{
			Number: number, EvidenceID: item.ID, CandidateID: item.CandidateID,
			DocumentID: item.DocumentID, Title: item.Title, URL: item.URL,
			ContentSHA256: item.ContentSHA256, ObservedAt: item.ObservedAt,
			Truncated: item.Truncated,
		})
	}
	if len(expectedSources) != len(artifact.Sources) {
		return fmt.Errorf("%w: completion source membership differs from cited evidence", ErrInvalidSynthesis)
	}
	for index := range expectedSources {
		if artifact.Sources[index] != expectedSources[index] {
			return fmt.Errorf("%w: completion source %d differs from acquired evidence", ErrInvalidSynthesis, index)
		}
	}
	rendered := renderArtifact(artifact.Paragraphs, expectedSources, numbers)
	digest := sha256.Sum256([]byte(rendered))
	if artifact.Rendered != rendered || artifact.SHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("%w: completion rendering differs from code-owned citation projection", ErrInvalidSynthesis)
	}
	return nil
}

func validateAcquiredArtifactEvidence(item Evidence) error {
	return websearch.ValidateDocument(websearch.Document{
		ID: item.DocumentID, CandidateID: item.CandidateID, URL: item.URL,
		Title: item.Title, Snippet: item.Snippet, Content: item.Content,
		ContentSHA256: item.ContentSHA256, ObservedAt: item.ObservedAt,
		Truncated: item.Truncated,
	})
}

func buildArtifact(
	paragraphs []GroundedParagraph,
	projected []ProjectedEvidence,
	allEvidence []Evidence,
	maxParagraphs, maxParagraphBytes int,
) (Artifact, error) {
	paragraphs, cited, err := validateSynthesis(
		paragraphs, projected, maxParagraphs, maxParagraphBytes,
	)
	if err != nil {
		return Artifact{}, err
	}
	sources := make([]CitationSource, 0, len(cited))
	numbers := make(map[EvidenceID]int, len(cited))
	for _, item := range allEvidence {
		if _, used := cited[item.ID]; !used {
			continue
		}
		number := len(sources) + 1
		numbers[item.ID] = number
		sources = append(sources, CitationSource{
			Number: number, EvidenceID: item.ID, CandidateID: item.CandidateID,
			DocumentID: item.DocumentID, Title: item.Title, URL: item.URL,
			ContentSHA256: item.ContentSHA256, ObservedAt: item.ObservedAt,
			Truncated: item.Truncated,
		})
	}
	if len(sources) != len(cited) {
		return Artifact{}, fmt.Errorf("%w: cited evidence is absent from authoritative acquisition", ErrInvalidSynthesis)
	}
	rendered := renderArtifact(paragraphs, sources, numbers)
	digest := sha256.Sum256([]byte(rendered))
	return Artifact{
		Paragraphs: cloneParagraphs(paragraphs), Sources: append([]CitationSource{}, sources...),
		Rendered: rendered, SHA256: hex.EncodeToString(digest[:]),
	}, nil
}

func validateSynthesis(
	decision []GroundedParagraph,
	projected []ProjectedEvidence,
	maxParagraphs, maxParagraphBytes int,
) ([]GroundedParagraph, map[EvidenceID]struct{}, error) {
	if len(decision) < 1 || len(decision) > maxParagraphs {
		return nil, nil, fmt.Errorf("%w: expected 1..%d paragraphs", ErrInvalidSynthesis, maxParagraphs)
	}
	available := make(map[EvidenceID]struct{}, len(projected))
	for _, item := range projected {
		available[item.EvidenceID] = struct{}{}
	}
	paragraphs := make([]GroundedParagraph, len(decision))
	allCited := make(map[EvidenceID]struct{})
	for index, paragraph := range decision {
		if paragraph.Text == "" || paragraph.Text != strings.TrimSpace(paragraph.Text) || len(paragraph.Text) > maxParagraphBytes {
			return nil, nil, fmt.Errorf("%w: paragraph %d must be trimmed and contain 1..%d bytes", ErrInvalidSynthesis, index, maxParagraphBytes)
		}
		if !utf8.ValidString(paragraph.Text) || strings.ContainsRune(paragraph.Text, '\x00') {
			return nil, nil, fmt.Errorf("%w: paragraph %d must be valid UTF-8 without NUL", ErrInvalidSynthesis, index)
		}
		if len(paragraph.EvidenceIDs) < 1 || len(paragraph.EvidenceIDs) > maxEvidenceIDsPerParagraph {
			return nil, nil, fmt.Errorf("%w: paragraph %d requires 1..%d evidence IDs", ErrInvalidSynthesis, index, maxEvidenceIDsPerParagraph)
		}
		if modelCitationSyntax.MatchString(paragraph.Text) {
			return nil, nil, fmt.Errorf("%w: paragraph %d contains model-authored citation syntax", ErrInvalidSynthesis, index)
		}
		seen := make(map[EvidenceID]struct{}, len(paragraph.EvidenceIDs))
		for _, id := range paragraph.EvidenceIDs {
			if _, exists := available[id]; !exists {
				return nil, nil, fmt.Errorf("%w: paragraph %d cites unknown evidence ID %q", ErrInvalidSynthesis, index, id)
			}
			if _, duplicate := seen[id]; duplicate {
				return nil, nil, fmt.Errorf("%w: paragraph %d duplicates evidence ID %q", ErrInvalidSynthesis, index, id)
			}
			seen[id] = struct{}{}
			allCited[id] = struct{}{}
		}
		paragraphs[index] = GroundedParagraph{Text: paragraph.Text, EvidenceIDs: append([]EvidenceID{}, paragraph.EvidenceIDs...)}
	}
	return paragraphs, allCited, nil
}

func renderArtifact(paragraphs []GroundedParagraph, sources []CitationSource, numbers map[EvidenceID]int) string {
	var builder strings.Builder
	for paragraphIndex, paragraph := range paragraphs {
		if paragraphIndex > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(paragraph.Text)
		citationNumbers := make([]int, 0, len(paragraph.EvidenceIDs))
		for _, id := range paragraph.EvidenceIDs {
			citationNumbers = append(citationNumbers, numbers[id])
		}
		sort.Ints(citationNumbers)
		for _, number := range citationNumbers {
			fmt.Fprintf(&builder, " [%d]", number)
		}
	}
	builder.WriteString("\n\nSources:\n")
	for _, source := range sources {
		title := source.Title
		if strings.TrimSpace(title) == "" {
			title = source.URL
		}
		fmt.Fprintf(&builder, "[%d] %s — %s\n", source.Number, title, source.URL)
	}
	return strings.TrimSpace(builder.String())
}
