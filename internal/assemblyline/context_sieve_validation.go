package assemblyline

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	MaxContextSearchTerms                 = 3
	MaxContextSearchTermBytes             = 256
	MaxContextCandidateAuthorities        = 16
	MaxContextCandidateNamespaceBytes     = 48
	MaxContextCandidateIDBytes            = 64
	MaxContextCandidateContentBytes       = 2 * 1024
	MaxContextCandidateProjectionBytes    = 8 * 1024
	MaxContextRelevanceSelections         = 8
	MaxContextMinificationAuthorities     = 8
	MaxContextMinificationProjectionBytes = 6 * 1024
	MaxContextMinifiedBytes               = 2 * 1024
)

var (
	contextNamespacePattern   = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,47}$`)
	contextCandidateIDPattern = regexp.MustCompile(`^CTX_[1-9][0-9]{0,5}$`)
)

// ContextCandidateAuthority is one exact, code-acquired candidate. Namespace
// identifies its code-owned authority class; CandidateID is the only value a
// relevance station may return.
type ContextCandidateAuthority struct {
	Namespace     string `json:"namespace"`
	CandidateID   string `json:"candidate_id"`
	Content       string `json:"content"`
	ContentSHA256 string `json:"content_sha256"`
}

func NewContextCandidateAuthority(
	namespace string,
	candidateID string,
	content string,
) (ContextCandidateAuthority, error) {
	authority := ContextCandidateAuthority{
		Namespace: namespace, CandidateID: candidateID, Content: content,
		ContentSHA256: ExactObjectiveContextSHA(content),
	}
	if err := validateContextCandidateAuthorities(
		"context candidate", []ContextCandidateAuthority{authority}, 1,
		MaxContextCandidateProjectionBytes,
	); err != nil {
		return ContextCandidateAuthority{}, err
	}
	return authority, nil
}

func validateContextExactInstruction(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("context sieve requires one non-blank exact instruction")
	}
	if len(value) > maxConversationInstructionBytes {
		return fmt.Errorf("context sieve exact instruction exceeds %d bytes", maxConversationInstructionBytes)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("context sieve exact instruction is not valid UTF-8")
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("context sieve exact instruction contains NUL")
	}
	return nil
}

func validateCanonicalContextRetrievalConcepts(concepts []string) error {
	if len(concepts) < 1 || len(concepts) > MaxContextSearchTerms {
		return fmt.Errorf(
			"context relevance requires 1..%d canonical retrieval concepts",
			MaxContextSearchTerms,
		)
	}
	previous := ""
	for index, concept := range concepts {
		if err := validateContextSearchTerm(concept); err != nil {
			return fmt.Errorf("context retrieval concept %d: %w", index, err)
		}
		if concept != strings.ToLower(concept) {
			return fmt.Errorf("context retrieval concept %d must be case-folded", index)
		}
		if index > 0 && concept <= previous {
			return fmt.Errorf("context retrieval concepts must be sorted and unique")
		}
		previous = concept
	}
	return nil
}

func validateContextCandidateAuthorities(
	label string,
	authorities []ContextCandidateAuthority,
	maximumCount int,
	maximumBytes int,
) error {
	if len(authorities) < 1 || len(authorities) > maximumCount {
		return fmt.Errorf("%s requires 1..%d candidate authorities", label, maximumCount)
	}
	seenIDs := make(map[string]struct{}, len(authorities))
	seenContent := make(map[string]struct{}, len(authorities))
	total := 0
	for index, authority := range authorities {
		if len(authority.Namespace) > MaxContextCandidateNamespaceBytes ||
			!contextNamespacePattern.MatchString(authority.Namespace) {
			return fmt.Errorf("%s candidate %d has invalid namespace %q", label, index, authority.Namespace)
		}
		if len(authority.CandidateID) > MaxContextCandidateIDBytes ||
			!contextCandidateIDPattern.MatchString(authority.CandidateID) {
			return fmt.Errorf("%s candidate %d has invalid opaque ID %q", label, index, authority.CandidateID)
		}
		if _, duplicate := seenIDs[authority.CandidateID]; duplicate {
			return fmt.Errorf("%s candidate ID %q is duplicated", label, authority.CandidateID)
		}
		seenIDs[authority.CandidateID] = struct{}{}
		if err := validateContextCandidateText(
			label+" candidate content", authority.Content, MaxContextCandidateContentBytes,
		); err != nil {
			return fmt.Errorf("candidate %s: %w", authority.CandidateID, err)
		}
		if !exactObjectiveContextSHA(authority.Content, authority.ContentSHA256) {
			return fmt.Errorf("%s candidate %s content hash does not match", label, authority.CandidateID)
		}
		if _, duplicate := seenContent[authority.ContentSHA256]; duplicate {
			return fmt.Errorf("%s candidate %s duplicates exact candidate content", label, authority.CandidateID)
		}
		seenContent[authority.ContentSHA256] = struct{}{}
		total += len(authority.Namespace) + len(authority.CandidateID) +
			len(authority.Content) + len(authority.ContentSHA256)
	}
	if total > maximumBytes {
		return fmt.Errorf("%s candidate projection exceeds %d bytes", label, maximumBytes)
	}
	return nil
}

func validateContextCandidateText(label, value string, maximum int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must be non-blank exact text", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s exceeds %d bytes", label, maximum)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", label)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s contains NUL", label)
	}
	return nil
}

func validateContextText(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must be non-empty trimmed text", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s exceeds %d bytes", label, maximum)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", label)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s contains NUL", label)
	}
	return nil
}

func validateOptionalContextText(label, value string, maximum int) error {
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must be trimmed text", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s exceeds %d bytes", label, maximum)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", label)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s contains NUL", label)
	}
	return nil
}

func decodeContextSieveDecision[T any](label, raw string) (T, error) {
	var decision T
	if len(raw) > maxPortableCandidateBytes {
		return decision, fmt.Errorf("%s candidate exceeds %d bytes", label, maxPortableCandidateBytes)
	}
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return decision, fmt.Errorf("decode %s decision: %w", label, err)
	}
	return decision, nil
}
