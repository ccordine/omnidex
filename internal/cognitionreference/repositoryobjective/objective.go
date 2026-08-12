package repositoryobjective

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxIdentityBytes = 128
	maxLookupBytes   = 512
	maxQuestionBytes = 2 * 1024
)

var acceptanceOrder = []AcceptancePredicate{
	AcceptanceSubjectResolved,
	AcceptanceDeclarationObserved,
	AcceptanceDirectRelationsKnown,
	AcceptanceApplicableTestsKnown,
}

func (objective Objective) validate() error {
	if err := validateIdentity(string(objective.ID), "objective ID"); err != nil {
		return err
	}
	if strings.TrimSpace(objective.Root) == "" || !filepath.IsAbs(objective.Root) {
		return fmt.Errorf("%w: root must be an absolute path", ErrInvalidObjective)
	}
	if filepath.Clean(objective.Root) != objective.Root {
		return fmt.Errorf("%w: root must be one clean absolute path", ErrInvalidObjective)
	}
	if objective.Subject.Kind != LookupQualifiedName && objective.Subject.Kind != LookupName {
		return fmt.Errorf("%w: unsupported lookup kind %q", ErrInvalidObjective, objective.Subject.Kind)
	}
	if err := validateText(objective.Subject.Value, maxLookupBytes, "subject lookup"); err != nil {
		return err
	}
	if objective.Question != "" {
		if err := validateText(objective.Question, maxQuestionBytes, "question"); err != nil {
			return err
		}
	}
	if len(objective.Acceptance) != len(acceptanceOrder) {
		return fmt.Errorf("%w: acceptance requires the exact registered predicate set", ErrInvalidObjective)
	}
	previous := -1
	for _, predicate := range objective.Acceptance {
		position := acceptancePosition(predicate)
		if position < 0 {
			return fmt.Errorf("%w: unsupported acceptance predicate %q", ErrInvalidObjective, predicate)
		}
		if position <= previous {
			return fmt.Errorf("%w: acceptance predicates must be unique and canonical", ErrInvalidObjective)
		}
		previous = position
	}
	if previous != len(acceptanceOrder)-1 {
		return fmt.Errorf("%w: acceptance requires the exact registered predicate set", ErrInvalidObjective)
	}
	return nil
}

func acceptancePosition(predicate AcceptancePredicate) int {
	for index, registered := range acceptanceOrder {
		if predicate == registered {
			return index
		}
	}
	return -1
}

func validateIdentity(value, label string) error {
	if value == "" || len(value) > maxIdentityBytes || !utf8.ValidString(value) ||
		strings.ContainsAny(value, "\x00 \t\r\n") {
		return fmt.Errorf("%w: %s must be nonempty bounded UTF-8 without whitespace", ErrInvalidObjective, label)
	}
	return nil
}

func validateText(value string, maximum int, label string) error {
	if value == "" || len(value) > maximum || !utf8.ValidString(value) ||
		strings.ContainsRune(value, 0) || strings.TrimSpace(value) != value {
		return fmt.Errorf("%w: %s must be nonempty bounded exact UTF-8", ErrInvalidObjective, label)
	}
	return nil
}

func cloneAcceptance(values []AcceptancePredicate) []AcceptancePredicate {
	return append([]AcceptancePredicate{}, values...)
}
