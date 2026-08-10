package cognition

import "fmt"

type CompletionCheckID string

type CompletionCheckRef struct {
	ID      CompletionCheckID `json:"id"`
	Version string            `json:"version"`
	SHA256  string            `json:"sha256"`
}

func (ref CompletionCheckRef) Validate() error {
	if err := validateIdentity(string(ref.ID), "completion check ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCompletionCheck, err)
	}
	if err := validateVersion(ref.Version, "completion check version"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCompletionCheck, err)
	}
	if !validSHA256(ref.SHA256) {
		return fmt.Errorf("%w: hash must be 64 lowercase hex characters", ErrInvalidCompletionCheck)
	}
	return nil
}

type CompletionOutcome string

const (
	CompletionUnsatisfied CompletionOutcome = "unsatisfied"
	CompletionSatisfied   CompletionOutcome = "satisfied"
)

type CompletionResult struct {
	ObligationID ObligationID       `json:"obligation_id"`
	Check        CompletionCheckRef `json:"check"`
	Revision     WorldRevision      `json:"revision"`
	Outcome      CompletionOutcome  `json:"outcome"`
	EvidenceRefs []EvidenceRef      `json:"evidence_refs"`
}

func NewCompletionResult(
	obligationID ObligationID,
	check CompletionCheckRef,
	revision WorldRevision,
	outcome CompletionOutcome,
	evidenceRefs []EvidenceRef,
) (CompletionResult, error) {
	result := CompletionResult{
		ObligationID: obligationID, Check: check, Revision: revision,
		Outcome: outcome, EvidenceRefs: cloneSlice(evidenceRefs),
	}
	if err := result.Validate(); err != nil {
		return CompletionResult{}, err
	}
	return result, nil
}

func (result CompletionResult) Validate() error {
	if err := validateIdentity(string(result.ObligationID), "completion obligation ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCompletionResult, err)
	}
	if err := result.Check.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCompletionResult, err)
	}
	if err := result.Revision.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCompletionResult, err)
	}
	switch result.Outcome {
	case CompletionUnsatisfied:
	case CompletionSatisfied:
		if len(result.EvidenceRefs) == 0 {
			return fmt.Errorf("%w: satisfied result requires exact evidence", ErrInvalidCompletionResult)
		}
	default:
		return fmt.Errorf("%w: outcome %q is not registered", ErrInvalidCompletionResult, result.Outcome)
	}
	if err := validateEvidenceRefs(result.EvidenceRefs); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCompletionResult, err)
	}
	return nil
}

func (result CompletionResult) ValidateFor(
	obligation Obligation,
	current WorldRevision,
	available []EvidenceRef,
) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if result.ObligationID != obligation.ID || result.Check != obligation.CompletionCheck {
		return fmt.Errorf("%w: result does not bind the obligation's registered check", ErrInvalidCompletionResult)
	}
	if result.Revision != current {
		return fmt.Errorf("%w: result does not bind the current revision", ErrInvalidCompletionResult)
	}
	known := make(map[string]struct{}, len(available))
	for _, ref := range available {
		known[evidenceIdentity(ref)] = struct{}{}
	}
	for index, ref := range result.EvidenceRefs {
		if ref.Revision.EpisodeID != current.EpisodeID || ref.Revision.Number > current.Number {
			return fmt.Errorf("%w: evidence %d is unavailable at the checked revision", ErrInvalidCompletionResult, index)
		}
		if _, exists := known[evidenceIdentity(ref)]; !exists {
			return fmt.Errorf("%w: evidence %d is absent from the code-owned packet", ErrInvalidCompletionResult, index)
		}
	}
	return nil
}

func (result CompletionResult) Clone() CompletionResult {
	result.EvidenceRefs = cloneSlice(result.EvidenceRefs)
	return result
}
