package assemblyline

import (
	"errors"
	"fmt"
)

type acceptanceGroundingCorrectionState struct {
	identity string
	rank     int
	fields   map[string]struct{}
}

func (state acceptanceGroundingCorrectionState) allows(field string) bool {
	_, exists := state.fields[field]
	return exists
}

func applicationAcceptanceGroundingCorrectionState(
	job PortableJob,
	retained string,
) (acceptanceGroundingCorrectionState, error) {
	var input ApplicationAcceptanceGroundingReviewInput
	if job.Kind != WorkApplicationAcceptanceGroundingReview {
		return acceptanceGroundingCorrectionState{}, fmt.Errorf(
			"acceptance grounding correction requires its original review job",
		)
	}
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return acceptanceGroundingCorrectionState{}, fmt.Errorf(
			"decode acceptance grounding correction authority: %w", err,
		)
	}
	if err := input.validate(); err != nil {
		return acceptanceGroundingCorrectionState{}, err
	}
	values, err := decodeApplicationAcceptanceGroundingLeaves(input, retained)
	if err != nil {
		var leafErr *ApplicationAcceptanceGroundingLeafValidationError
		if errors.As(err, &leafErr) {
			return acceptanceGroundingCorrectionState{
				identity: "leaf:" + leafErr.Field(), rank: 0,
				fields: map[string]struct{}{leafErr.Field(): {}},
			}, nil
		}
		return acceptanceGroundingCorrectionState{}, err
	}
	review := deriveApplicationAcceptanceGroundingReview(input, values)
	switch {
	case review.Decision == AcceptanceGroundingAccept:
		return acceptanceGroundingCorrectionState{
			identity: "accepted", rank: 3, fields: map[string]struct{}{},
		}, nil
	case review.UnsupportedSiteID != "":
		return acceptanceGroundingCorrectionState{
			identity: "unsupported:" + review.UnsupportedSiteID, rank: 1,
			fields: map[string]struct{}{},
		}, nil
	case review.MissingCriterionID != "":
		return acceptanceGroundingCorrectionState{
			identity: "missing:" + review.MissingCriterionID, rank: 2,
			fields: map[string]struct{}{},
		}, nil
	}
	return acceptanceGroundingCorrectionState{}, fmt.Errorf(
		"acceptance grounding correction state has no code-derived decision",
	)
}
