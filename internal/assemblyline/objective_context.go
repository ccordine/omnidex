package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxMemoryContextCandidateAuthorities = 8
	MaxMemoryContextCandidateBytes       = 6 * 1024
	MaxObjectiveContextCapsules          = 1
	// MaxObjectiveControlFeedbackBytes retains the narrow explicit lifecycle
	// command boundary. Ordinary session text has the same 4 KiB authority
	// whether it starts a job or deterministically replans the active job.
	MaxObjectiveControlFeedbackBytes = 2 * 1024
	MaxObjectiveReplanFeedbackBytes  = 4 * 1024
	// MaxObjectiveContextAuthorityBytes is a coarse portable-payload safety
	// limit, not a semantic projection target. Per-call context budgets drive
	// paging and staged reduction before this resource boundary is reached.
	MaxObjectiveContextAuthorityBytes = 128 * 1024
)

// ObjectiveReplanAuthority is the exact feedback attached to the current
// generation of this same job. It is sibling authority, not a rewritten user
// instruction.
type ObjectiveReplanAuthority struct {
	JobID          int64  `json:"job_id"`
	Generation     int64  `json:"generation"`
	Feedback       string `json:"feedback"`
	FeedbackSHA256 string `json:"feedback_sha256"`
}

// ObjectiveContextSource binds one compiled capsule back to the exact
// code-acquired authority selected by the relevance sieve. Source bytes are
// intentionally absent: downstream models receive only the compiled capsule.
type ObjectiveContextSource struct {
	Namespace     string `json:"namespace"`
	CandidateID   string `json:"candidate_id"`
	ContentSHA256 string `json:"content_sha256"`
}

// ObjectiveContextCapsule is the only historical/contextual prose projected
// to downstream objective stations. Its text is either selected authority
// preserved verbatim or the result of necessary staged semantic reduction.
// Code owns its sources, hash, ordering, and resource bounds.
type ObjectiveContextCapsule struct {
	Sources       []ObjectiveContextSource `json:"sources"`
	Content       string                   `json:"content"`
	ContentSHA256 string                   `json:"content_sha256"`
}

// ObjectiveContext is the sole bounded continuity projection shared by the
// objective classifier and every non-coding response/evidence station. Raw
// transcript turns, memory rows, and fictional archives are never fields of
// this model-visible projection.
type ObjectiveContext struct {
	Capsules        []ObjectiveContextCapsule `json:"capsules"`
	ReplanAuthority *ObjectiveReplanAuthority `json:"replan_authority"`
}

// renderObjectiveContextForModel exposes only accepted context prose. Source
// identities, hashes, and portable state remain code-owned.
func renderObjectiveContextForModel(context ObjectiveContext) (string, error) {
	if err := context.Validate(); err != nil {
		return "", err
	}
	contents := make([]string, len(context.Capsules))
	for index, capsule := range context.Capsules {
		contents[index] = capsule.Content
	}
	return strings.Join(contents, "\n\n"), nil
}

func (context ObjectiveContext) Validate() error {
	if len(context.Capsules) > MaxObjectiveContextCapsules {
		return fmt.Errorf("objective context exceeds the %d-capsule bound", MaxObjectiveContextCapsules)
	}
	seenSources := make(map[string]struct{})
	total := 0
	for capsuleIndex, capsule := range context.Capsules {
		if len(capsule.Sources) < 1 {
			return fmt.Errorf("objective context capsule %d requires exact sources", capsuleIndex)
		}
		for sourceIndex, source := range capsule.Sources {
			if !contextNamespacePattern.MatchString(source.Namespace) {
				return fmt.Errorf("objective context capsule %d source %d has invalid namespace", capsuleIndex, sourceIndex)
			}
			if !contextCandidateIDPattern.MatchString(source.CandidateID) {
				return fmt.Errorf("objective context capsule %d source %d has invalid candidate ID", capsuleIndex, sourceIndex)
			}
			if _, duplicate := seenSources[source.CandidateID]; duplicate {
				return fmt.Errorf("objective context source %q is duplicated", source.CandidateID)
			}
			seenSources[source.CandidateID] = struct{}{}
			if len(source.ContentSHA256) != 64 {
				return fmt.Errorf("objective context source %q has invalid content hash", source.CandidateID)
			}
			if _, err := hex.DecodeString(source.ContentSHA256); err != nil || source.ContentSHA256 != strings.ToLower(source.ContentSHA256) {
				return fmt.Errorf("objective context source %q has invalid content hash", source.CandidateID)
			}
			total += len(source.Namespace) + len(source.CandidateID) + len(source.ContentSHA256)
		}
		if err := validateObjectiveContextText("context capsule", capsule.Content, MaxContextMinifiedBytes); err != nil {
			return fmt.Errorf("objective context capsule %d: %w", capsuleIndex, err)
		}
		if !exactObjectiveContextSHA(capsule.Content, capsule.ContentSHA256) {
			return fmt.Errorf("objective context capsule %d content hash does not match", capsuleIndex)
		}
		total += len(capsule.Content) + len(capsule.ContentSHA256)
	}
	if context.ReplanAuthority != nil {
		replan := context.ReplanAuthority
		if replan.JobID < 1 || replan.Generation < 2 {
			return fmt.Errorf("objective replan authority requires one same-job generation after the initial generation")
		}
		if err := validateObjectiveContextText(
			"replan feedback", replan.Feedback, MaxObjectiveReplanFeedbackBytes,
		); err != nil {
			return err
		}
		if !exactObjectiveContextSHA(replan.Feedback, replan.FeedbackSHA256) {
			return fmt.Errorf("objective replan feedback hash does not match")
		}
		total += len(replan.Feedback)
	}
	if total > MaxObjectiveContextAuthorityBytes {
		return fmt.Errorf(
			"objective context authority exceeds the %d-byte portable resource limit",
			MaxObjectiveContextAuthorityBytes,
		)
	}
	return nil
}

func CloneObjectiveContext(value ObjectiveContext) ObjectiveContext {
	value.Capsules = append([]ObjectiveContextCapsule(nil), value.Capsules...)
	for index := range value.Capsules {
		value.Capsules[index].Sources = append(
			[]ObjectiveContextSource(nil), value.Capsules[index].Sources...,
		)
	}
	if value.ReplanAuthority != nil {
		copy := *value.ReplanAuthority
		value.ReplanAuthority = &copy
	}
	return value
}

func ExactObjectiveContextSHA(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func exactObjectiveContextSHA(value, expected string) bool {
	return expected == ExactObjectiveContextSHA(value)
}

func validateObjectiveContextText(label, value string, maximum int) error {
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > maximum {
		return fmt.Errorf("objective %s must be exact non-blank UTF-8 text of at most %d bytes", label, maximum)
	}
	return nil
}
