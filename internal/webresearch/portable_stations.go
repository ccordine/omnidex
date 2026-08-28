package webresearch

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

const (
	maxPortableQuestionBytes           = 4 * 1024
	maxPortableQueries                 = 4
	maxPortableQueryBytes              = 1024
	maxPortableSearchTerms             = 3
	maxPortableSearchTermBytes         = 256
	maxPortableCandidates              = 32
	maxPortableCandidateProjection     = 8 * 1024
	maxPortableEvidence                = 32
	maxPortableEvidenceProjection      = 8 * 1024
	maxPortableStationIdentityBytes    = 128
	maxPortableCandidateFieldBytes     = 2 * 1024
	maxPortableEvidenceFieldBytes      = 8 * 1024
	maxPortableSynthesisParagraphs     = 4
	maxPortableSynthesisParagraphBytes = 2 * 1024
	maxPortableReviewDetailBytes       = 512
	maxPortableReviewEvidence          = 4
)

type PortableExecutor func(
	context.Context,
	assemblyline.PortableJob,
) (assemblyline.PortableResult, error)

type PortableFinalizer func(
	context.Context,
	assemblyline.PortableJob,
	assemblyline.PortableResult,
	error,
) error

type PortableRuntime struct {
	Execute  PortableExecutor
	Finalize PortableFinalizer
}

type PortableStations struct {
	runtime PortableRuntime
}

func NewPortableStations(runtime PortableRuntime) (*PortableStations, error) {
	if runtime.Execute == nil || runtime.Finalize == nil {
		return nil, fmt.Errorf("portable web stations require one exact runtime")
	}
	return &PortableStations{runtime: runtime}, nil
}

func validatePortableRelevanceCall(call RelevanceCall) error {
	if err := validatePortableQuestion(call.Question); err != nil {
		return err
	}
	if err := validatePortableObjectiveContext(call.Question, call.Context); err != nil {
		return err
	}
	if len(call.Candidates) < 1 || len(call.Candidates) > maxPortableCandidates {
		return fmt.Errorf("portable relevance requires 1..%d candidates", maxPortableCandidates)
	}
	if call.MaxSelections < 1 || call.MaxSelections > len(call.Candidates) {
		return fmt.Errorf("portable relevance selection bound must fit candidates")
	}
	total := 0
	seen := make(map[websearch.CandidateID]struct{}, len(call.Candidates))
	for _, candidate := range call.Candidates {
		if err := validatePortableIdentity(string(candidate.CandidateID)); err != nil {
			return err
		}
		if _, duplicate := seen[candidate.CandidateID]; duplicate {
			return fmt.Errorf("portable relevance candidate ID %q is duplicated", candidate.CandidateID)
		}
		seen[candidate.CandidateID] = struct{}{}
		if strings.TrimSpace(candidate.Excerpt) == "" {
			return fmt.Errorf("portable relevance candidate %q has no excerpt", candidate.CandidateID)
		}
		total += len(candidate.CandidateID) + len(candidate.Title) + len(candidate.Snippet) + len(candidate.Excerpt)
		if total > maxPortableCandidateProjection {
			return fmt.Errorf("portable relevance projection exceeds %d bytes", maxPortableCandidateProjection)
		}
		for _, value := range []string{candidate.Title, candidate.Snippet, candidate.Excerpt} {
			if err := validatePortableField(value, maxPortableCandidateFieldBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePortableSearchTermsCall(call SearchTermsCall) error {
	if err := validatePortableQuestion(call.Question); err != nil {
		return err
	}
	if err := validatePortableObjectiveContext(call.Question, call.Context); err != nil {
		return err
	}
	if len(call.AttemptedQueries) > maxPortableQueries {
		return fmt.Errorf("portable search terms allow at most %d attempted queries", maxPortableQueries)
	}
	if call.MaxTerms < 1 || call.MaxTerms > maxPortableSearchTerms {
		return fmt.Errorf("portable search-term count bound must be 1..%d", maxPortableSearchTerms)
	}
	if call.MaxTermBytes < 1 || call.MaxTermBytes > maxPortableSearchTermBytes {
		return fmt.Errorf("portable search-term byte bound must be 1..%d", maxPortableSearchTermBytes)
	}
	seen := make(map[string]struct{}, len(call.AttemptedQueries))
	for _, query := range call.AttemptedQueries {
		if err := validatePortableIdentityBound(query, maxPortableQueryBytes); err != nil {
			return err
		}
		identity := strings.ToLower(query)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("portable attempted query %q is duplicated", query)
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func validatePortableSynthesisCall(call GroundedSynthesisCall) error {
	if err := validatePortableQuestion(call.Question); err != nil {
		return err
	}
	if err := validatePortableObjectiveContext(call.Question, call.Context); err != nil {
		return err
	}
	if len(call.Evidence) < 1 || len(call.Evidence) > maxPortableEvidence {
		return fmt.Errorf("portable synthesis requires 1..%d evidence capsules", maxPortableEvidence)
	}
	if call.MaxParagraphs < 1 || call.MaxParagraphs > maxPortableSynthesisParagraphs {
		return fmt.Errorf("portable synthesis paragraph count bound must be 1..%d", maxPortableSynthesisParagraphs)
	}
	if call.MaxParagraphBytes < 1 || call.MaxParagraphBytes > maxPortableSynthesisParagraphBytes {
		return fmt.Errorf("portable synthesis paragraph byte bound must be 1..%d", maxPortableSynthesisParagraphBytes)
	}
	total := 0
	seen := make(map[EvidenceID]struct{}, len(call.Evidence))
	for _, item := range call.Evidence {
		if err := validatePortableIdentity(string(item.EvidenceID)); err != nil {
			return err
		}
		if _, duplicate := seen[item.EvidenceID]; duplicate {
			return fmt.Errorf("portable synthesis evidence ID %q is duplicated", item.EvidenceID)
		}
		seen[item.EvidenceID] = struct{}{}
		if strings.TrimSpace(item.Content) == "" {
			return fmt.Errorf("portable synthesis evidence %q has no content", item.EvidenceID)
		}
		total += len(item.EvidenceID) + len(item.Title) + len(item.Snippet) + len(item.Content)
		if total > maxPortableEvidenceProjection {
			return fmt.Errorf("portable synthesis projection exceeds %d bytes", maxPortableEvidenceProjection)
		}
		for _, value := range []string{item.Title, item.Snippet, item.Content} {
			if err := validatePortableField(value, maxPortableEvidenceFieldBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePortableObjectiveContext(question string, context assemblyline.ObjectiveContext) error {
	_, err := assemblyline.NewConversationObjectiveKindJob(assemblyline.ConversationObjectiveKindInput{
		ExactInstruction: question,
		Context:          context,
	})
	return err
}

func validatePortableQuestion(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > maxPortableQuestionBytes || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("portable web question must be exact non-blank UTF-8 without NUL within %d bytes", maxPortableQuestionBytes)
	}
	return nil
}

func validatePortableIdentity(value string) error {
	return validatePortableIdentityBound(value, maxPortableStationIdentityBytes)
}

func validatePortableIdentityBound(value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maximum || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("portable web identity must be one bounded trimmed line")
	}
	return validatePortableField(value, maximum)
}

func validatePortableField(value string, maximum int) error {
	if len(value) > maximum || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("portable web field is invalid or exceeds %d bytes", maximum)
	}
	return nil
}
