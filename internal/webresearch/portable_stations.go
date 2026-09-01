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
	maxPortableCandidates              = 32
	maxPortableCandidateProjection     = 8 * 1024
	maxPortableEvidence                = 32
	maxPortableStationIdentityBytes    = 128
	maxPortableCandidateFieldBytes     = 2 * 1024
	maxPortableSynthesisParagraphs     = 4
	maxPortableSynthesisParagraphBytes = 2 * 1024
)

type PortableCandidateValidator func(string) error

type PortableResolver func(
	context.Context,
	assemblyline.PortableJob,
	PortableCandidateValidator,
) (SemanticCallReceipt, error)

type PortableRuntime struct {
	Resolve PortableResolver
}

type PortableStations struct {
	runtime PortableRuntime
}

func NewPortableStations(runtime PortableRuntime) (*PortableStations, error) {
	if runtime.Resolve == nil {
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
