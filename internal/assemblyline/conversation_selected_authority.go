package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxConversationContextCandidateAuthorities = 8
	MaxConversationContextCandidateBytes       = 6 * 1024
	MaxSelectedConversationProjectionBytes     = 2 * 1024
)

// ConversationSelectedUserAuthority is immutable user-authored text selected
// by ID from a code-owned candidate set. Models cannot create or rewrite it.
type ConversationSelectedUserAuthority struct {
	MessageID int64  `json:"message_id"`
	Content   string `json:"content"`
}

// ConversationSelectedAssistantResult is a server-produced result whose
// exact channel job and user-message anchor were proven by code. It is not
// user authority and must never be relabeled as one.
type ConversationSelectedAssistantResult struct {
	UserMessageID int64  `json:"user_message_id"`
	MessageID     int64  `json:"message_id"`
	JobID         int64  `json:"job_id"`
	Content       string `json:"content"`
}

func validateConversationSelectedUserAuthorities(
	authorities []ConversationSelectedUserAuthority,
) error {
	if len(authorities) > MaxConversationContextCandidateAuthorities {
		return fmt.Errorf(
			"selected conversation authorities exceed the %d-authority bound",
			MaxConversationContextCandidateAuthorities,
		)
	}
	total := 0
	var previousID int64
	for index, authority := range authorities {
		if authority.MessageID < 1 || authority.MessageID <= previousID {
			return fmt.Errorf("selected conversation authority %d has invalid message order", index)
		}
		if strings.TrimSpace(authority.Content) == "" || !utf8.ValidString(authority.Content) ||
			strings.ContainsRune(authority.Content, '\x00') {
			return fmt.Errorf("selected conversation authority %d has invalid exact content", index)
		}
		total += len(authority.Content)
		previousID = authority.MessageID
	}
	if total > MaxSelectedConversationProjectionBytes {
		return fmt.Errorf(
			"selected conversation authority exceeds %d bytes",
			MaxSelectedConversationProjectionBytes,
		)
	}
	return nil
}

func validateConversationSelectedAssistantResults(
	users []ConversationSelectedUserAuthority,
	results []ConversationSelectedAssistantResult,
) error {
	if len(results) > len(users) {
		return fmt.Errorf("selected assistant results exceed selected user authorities")
	}
	userIDs := make(map[int64]struct{}, len(users))
	total := 0
	for _, authority := range users {
		userIDs[authority.MessageID] = struct{}{}
		total += len(authority.Content)
	}
	var previousUserID int64
	for index, result := range results {
		if _, ok := userIDs[result.UserMessageID]; !ok {
			return fmt.Errorf("selected assistant result %d has no selected user authority", index)
		}
		if result.UserMessageID <= previousUserID || result.MessageID <= result.UserMessageID || result.JobID < 1 {
			return fmt.Errorf("selected assistant result %d has invalid immutable identity", index)
		}
		if strings.TrimSpace(result.Content) == "" || !utf8.ValidString(result.Content) ||
			strings.ContainsRune(result.Content, '\x00') {
			return fmt.Errorf("selected assistant result %d has invalid exact content", index)
		}
		total += len(result.Content)
		previousUserID = result.UserMessageID
	}
	if total > MaxSelectedConversationProjectionBytes {
		return fmt.Errorf(
			"selected conversation projection exceeds %d bytes",
			MaxSelectedConversationProjectionBytes,
		)
	}
	return nil
}

func validateConversationSelectedProjection(
	users []ConversationSelectedUserAuthority,
	results []ConversationSelectedAssistantResult,
) error {
	if err := validateConversationSelectedUserAuthorities(users); err != nil {
		return err
	}
	return validateConversationSelectedAssistantResults(users, results)
}
