package taskstate

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxFeedbackEntryContentBytes = 64 * 1024

func validateLedgerOwner(owner LedgerOwner) error {
	if owner.Kind != OwnerJob {
		return fmt.Errorf("%w: ledger owner kind %q is not registered", ErrInvalidState, owner.Kind)
	}
	if owner.JobID <= 0 {
		return fmt.Errorf("%w: ledger job ID must be positive", ErrInvalidState)
	}
	if !uuidPattern.MatchString(owner.RunID) {
		return fmt.Errorf("%w: ledger run ID must be a lowercase UUID", ErrInvalidState)
	}
	return nil
}

func validateAuthority(authority Authority) error {
	switch authority {
	case AuthorityUser, AuthorityCode, AuthorityToolEvidence:
		return nil
	default:
		return fmt.Errorf("%w: authority %q is not registered", ErrInvalidCommand, authority)
	}
}

func requireCode(actor Authority, action string) error {
	if actor != AuthorityCode {
		return fmt.Errorf("%w: only code may %s", ErrAuthorityDenied, action)
	}
	return nil
}

func requireExactText(value, field string) error {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%w: %s must be one nonempty exact value", ErrInvalidCommand, field)
	}
	return nil
}

func requireEntryContent(value string, kind EntryKind) error {
	if kind != EntryFeedback {
		return requireExactText(value, "entry content")
	}
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" ||
		strings.ContainsRune(value, '\x00') || len(value) > maxFeedbackEntryContentBytes {
		return fmt.Errorf(
			"%w: feedback entry content must be one nonblank valid UTF-8 value within %d bytes",
			ErrInvalidCommand,
			maxFeedbackEntryContentBytes,
		)
	}
	return nil
}

func validateNodeKind(kind NodeKind) error {
	switch kind {
	case NodeGoal, NodeObjective, NodeTask, NodeCheckpoint, NodeChangeGroup:
		return nil
	default:
		return fmt.Errorf("%w: node kind %q is not registered", ErrInvalidCommand, kind)
	}
}

func validateEdgeKind(kind EdgeKind) error {
	switch kind {
	case EdgeDependsOn, EdgeBlocks, EdgeDecomposes, EdgeVerifies:
		return nil
	default:
		return fmt.Errorf("%w: edge kind %q is not registered", ErrInvalidCommand, kind)
	}
}

func validateEntryKind(kind EntryKind) error {
	switch kind {
	case EntryConstraint, EntryFact, EntryObservation, EntryHypothesis, EntryQuestion,
		EntryFailure, EntryCheckpoint, EntryNote, EntryFeedback:
		return nil
	default:
		return fmt.Errorf("%w: entry kind %q is not registered", ErrInvalidCommand, kind)
	}
}

func validatePriority(priority int) error {
	if priority < 1 || priority > 100 {
		return fmt.Errorf("%w: priority must be between 1 and 100", ErrInvalidCommand)
	}
	return nil
}

func validateOptionalStep(step *int64, field string) error {
	if step != nil && *step <= 0 {
		return fmt.Errorf("%w: %s must be positive", ErrInvalidCommand, field)
	}
	return nil
}

func validateCriteria(criteria []string) error {
	seen := make(map[string]struct{}, len(criteria))
	for _, criterion := range criteria {
		if err := requireExactText(criterion, "acceptance criterion"); err != nil {
			return err
		}
		if _, exists := seen[criterion]; exists {
			return fmt.Errorf("%w: acceptance criteria must be unique", ErrInvalidCommand)
		}
		seen[criterion] = struct{}{}
	}
	return nil
}

func validateRefs(refs []Ref) error {
	seen := make(map[string]string, len(refs))
	for index, ref := range refs {
		if err := ValidateRef(ref); err != nil {
			return fmt.Errorf("reference %d: %w", index, err)
		}
		key := RefIdentity(ref)
		if existingHash, exists := seen[key]; exists {
			if existingHash != ref.Hash {
				return fmt.Errorf("%w: reference identity at index %d conflicts with a different hash", ErrInvalidState, index)
			}
			return fmt.Errorf("%w: duplicate reference identity at index %d", ErrInvalidState, index)
		}
		seen[key] = ref.Hash
	}
	return nil
}

// ValidateRef validates the canonical stable-reference contract shared by task state
// and bounded attention systems.
func ValidateRef(ref Ref) error {
	if !utf8.ValidString(ref.URI) || !refURIPattern.MatchString(ref.URI) ||
		containsUnicodeWhitespace(ref.URI) || strings.ContainsRune(ref.URI, '\x00') {
		return fmt.Errorf("%w: reference URI requires a lowercase scheme and nonempty whitespace-free suffix", ErrInvalidCommand)
	}
	if err := requireExactText(ref.Version, "reference version"); err != nil {
		return err
	}
	if !validDigest(ref.Hash) {
		return fmt.Errorf("%w: reference hash must be 64 lowercase hex characters", ErrInvalidCommand)
	}
	switch ref.Relation {
	case RefEvidence, RefSource, RefSupports, RefContradicts,
		RefConcerns, RefVerifies, RefSupersedes:
		return nil
	default:
		return fmt.Errorf("%w: reference relation %q is not registered", ErrInvalidCommand, ref.Relation)
	}
}

func containsUnicodeWhitespace(value string) bool {
	for _, character := range value {
		if unicode.IsSpace(character) {
			return true
		}
	}
	return false
}

// RefIdentity returns the stable identity of a validated reference. The content hash
// is deliberately excluded: a different hash for this identity is stale or conflicting
// content, not another reference.
func RefIdentity(ref Ref) string {
	return ref.URI + "\x00" + ref.Version + "\x00" + string(ref.Relation)
}

func hasEvidenceRef(refs []Ref) bool {
	for _, ref := range refs {
		if ref.Relation == RefEvidence || ref.Relation == RefSupports || ref.Relation == RefVerifies {
			return true
		}
	}
	return false
}

func hasContradictionRef(refs []Ref) bool {
	for _, ref := range refs {
		if ref.Relation == RefContradicts {
			return true
		}
	}
	return false
}

func executableNode(kind NodeKind) bool {
	return kind == NodeTask || kind == NodeCheckpoint || kind == NodeChangeGroup
}

func terminalNode(status NodeStatus) bool {
	return status == NodeDone || status == NodeFailed || status == NodeCanceled
}

func terminalLedger(status LedgerStatus) bool {
	return status == LedgerClosed || status == LedgerFailed || status == LedgerCanceled
}
