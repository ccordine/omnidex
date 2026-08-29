package changeapply

import (
	"errors"
	"fmt"
)

type DeletionCandidateIneligibility string

const (
	DeletionCandidateIneligibleUnsupported          DeletionCandidateIneligibility = "unsupported_source"
	DeletionCandidateIneligibleUntracked            DeletionCandidateIneligibility = "untracked_source"
	DeletionCandidateIneligibleIgnored              DeletionCandidateIneligibility = "ignored_source"
	DeletionCandidateIneligibleGenerated            DeletionCandidateIneligibility = "generated_source"
	DeletionCandidateIneligibleDeclarationAuthority DeletionCandidateIneligibility = "declaration_authority"
	DeletionCandidateIneligibleBuildMembership      DeletionCandidateIneligibility = "build_membership"
	DeletionCandidateIneligibleRemainingReference   DeletionCandidateIneligibility = "remaining_reference"
)

type DeletionCandidateIneligibleError struct {
	FileID string
	Reason DeletionCandidateIneligibility
	Cause  error
}

func (err *DeletionCandidateIneligibleError) Error() string {
	if err == nil {
		return "repository deletion candidate is ineligible"
	}
	return fmt.Sprintf(
		"repository deletion candidate %q is ineligible (%s): %v",
		err.FileID, err.Reason, err.Cause,
	)
}

func (err *DeletionCandidateIneligibleError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func DeletionCandidateIneligibilityOf(
	err error,
) (DeletionCandidateIneligibility, bool) {
	var ineligible *DeletionCandidateIneligibleError
	if !errors.As(err, &ineligible) || ineligible == nil {
		return "", false
	}
	return ineligible.Reason, true
}

func deletionCandidateIneligible(
	fileID string,
	reason DeletionCandidateIneligibility,
	cause error,
) error {
	return &DeletionCandidateIneligibleError{
		FileID: fileID, Reason: reason, Cause: cause,
	}
}
