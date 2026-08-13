package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

const (
	MaxMemoryContextCandidateAuthorities = 8
	MaxMemoryContextCandidateBytes       = 6 * 1024
	MaxSelectedMemoryProjectionBytes     = 1024
	MaxObjectiveReplanFeedbackBytes      = 2 * 1024
	MaxObjectiveContextBytes             = 5 * 1024
)

// ObjectiveMemoryAuthority is an immutable capsule projected by code after
// an ID-only selection from an exact project/channel-scoped candidate set.
type ObjectiveMemoryAuthority struct {
	MemoryID      int64            `json:"memory_id"`
	Kind          model.MemoryKind `json:"kind"`
	Content       string           `json:"content"`
	ContentSHA256 string           `json:"content_sha256"`
}

// ObjectiveReplanAuthority is the exact feedback attached to the current
// generation of this same job. It is sibling authority, not a rewritten user
// instruction.
type ObjectiveReplanAuthority struct {
	JobID          int64  `json:"job_id"`
	Generation     int64  `json:"generation"`
	Feedback       string `json:"feedback"`
	FeedbackSHA256 string `json:"feedback_sha256"`
}

// ObjectiveContext is the sole bounded continuity projection shared by the
// objective classifier and every non-coding response/evidence station.
type ObjectiveContext struct {
	UserAuthorities   []ConversationSelectedUserAuthority   `json:"user_authorities"`
	AssistantResults  []ConversationSelectedAssistantResult `json:"assistant_results"`
	MemoryAuthorities []ObjectiveMemoryAuthority            `json:"memory_authorities"`
	ReplanAuthority   *ObjectiveReplanAuthority             `json:"replan_authority"`
}

func (context ObjectiveContext) Validate() error {
	if err := validateConversationSelectedProjection(
		context.UserAuthorities, context.AssistantResults,
	); err != nil {
		return err
	}
	if len(context.MemoryAuthorities) > MaxMemoryContextCandidateAuthorities {
		return fmt.Errorf("objective memory projection exceeds the %d-authority bound", MaxMemoryContextCandidateAuthorities)
	}
	seenMemory := make(map[int64]struct{}, len(context.MemoryAuthorities))
	total := 0
	memoryBytes := 0
	for index, authority := range context.MemoryAuthorities {
		if authority.MemoryID < 1 {
			return fmt.Errorf("objective memory authority %d has invalid identity", index)
		}
		if _, duplicate := seenMemory[authority.MemoryID]; duplicate {
			return fmt.Errorf("objective memory authority %d is duplicated", authority.MemoryID)
		}
		seenMemory[authority.MemoryID] = struct{}{}
		if _, err := model.ParseMemoryKind(string(authority.Kind)); err != nil {
			return fmt.Errorf("objective memory authority %d: %w", index, err)
		}
		if err := validateObjectiveContextText("memory content", authority.Content, model.MaxMemoryContentBytes); err != nil {
			return fmt.Errorf("objective memory authority %d: %w", index, err)
		}
		if !exactObjectiveContextSHA(authority.Content, authority.ContentSHA256) {
			return fmt.Errorf("objective memory authority %d content hash does not match", index)
		}
		memoryBytes += len(authority.Content)
	}
	if memoryBytes > MaxSelectedMemoryProjectionBytes {
		return fmt.Errorf("objective memory projection exceeds %d bytes", MaxSelectedMemoryProjectionBytes)
	}
	for _, authority := range context.UserAuthorities {
		total += len(authority.Content)
	}
	for _, result := range context.AssistantResults {
		total += len(result.Content)
	}
	total += memoryBytes
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
	if total > MaxObjectiveContextBytes {
		return fmt.Errorf("objective context exceeds %d exact content bytes", MaxObjectiveContextBytes)
	}
	return nil
}

func CloneObjectiveContext(value ObjectiveContext) ObjectiveContext {
	value.UserAuthorities = append([]ConversationSelectedUserAuthority(nil), value.UserAuthorities...)
	value.AssistantResults = append([]ConversationSelectedAssistantResult(nil), value.AssistantResults...)
	value.MemoryAuthorities = append([]ObjectiveMemoryAuthority(nil), value.MemoryAuthorities...)
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
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > maximum {
		return fmt.Errorf("objective %s must be exact non-empty UTF-8 text of at most %d bytes", label, maximum)
	}
	return nil
}
