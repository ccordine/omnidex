package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

type applicationJobSpecificationReviewPortablePayload struct {
	Authority         ApplicationJobSpecificationInput                `json:"authority"`
	Retained          ApplicationJobSpecification                     `json:"retained"`
	Field             ApplicationJobSpecificationField                `json:"field"`
	EvidenceID        string                                          `json:"evidence_id"`
	Attempt           int                                             `json:"attempt"`
	ValidationFailure *ApplicationJobSpecificationReviewEvidenceError `json:"validation_failure,omitempty"`
}

type applicationJobSpecificationRetainedAuthority struct {
	Authority ApplicationJobSpecificationInput `json:"authority"`
	Retained  ApplicationJobSpecification      `json:"retained"`
}

// DecodeApplicationJobSpecificationResult restores the frozen authority from
// one portable specification job and applies the production decoder and
// semantic validator to the untrusted final response.
func DecodeApplicationJobSpecificationResult(
	job PortableJob,
	raw string,
) (ApplicationJobSpecification, error) {
	var zero ApplicationJobSpecification
	if err := job.Validate(); err != nil {
		return zero, err
	}
	if job.Kind != WorkApplicationJobSpecification {
		return zero, fmt.Errorf(
			"application job specification result requires work kind %q",
			WorkApplicationJobSpecification,
		)
	}
	var input ApplicationJobSpecificationInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return zero, err
	}
	specification, err := DecodeApplicationJobSpecification(input, raw)
	if err != nil {
		return zero, err
	}
	if err := ValidateApplicationJobSpecification(specification); err != nil {
		return zero, err
	}
	return specification, nil
}

func newApplicationJobSpecificationReviewPortablePayload(
	input ApplicationJobSpecificationReviewInput,
) (applicationJobSpecificationReviewPortablePayload, error) {
	if err := input.validate(); err != nil {
		return applicationJobSpecificationReviewPortablePayload{}, err
	}
	payload := applicationJobSpecificationReviewPortablePayload{
		Authority:  input.authority,
		Retained:   cloneApplicationJobSpecification(input.retained),
		Field:      input.field,
		EvidenceID: input.evidenceID,
		Attempt:    input.attempt,
	}
	if input.validationFailure != nil {
		copy := *input.validationFailure
		payload.ValidationFailure = &copy
	}
	return payload, nil
}

func (payload applicationJobSpecificationReviewPortablePayload) validate() error {
	_, err := payload.reviewInput()
	return err
}

func (payload applicationJobSpecificationReviewPortablePayload) reviewInput() (
	ApplicationJobSpecificationReviewInput,
	error,
) {
	if payload.ValidationFailure == nil {
		return NewApplicationJobSpecificationReviewInput(
			payload.Authority, payload.Retained, payload.Field, payload.EvidenceID,
			payload.Attempt,
		)
	}
	return NewApplicationJobSpecificationReviewRetryInput(
		payload.Authority, payload.Retained, payload.Field, payload.EvidenceID,
		payload.Attempt,
		*payload.ValidationFailure,
	)
}

func renderApplicationJobSpecificationReviewPortable(
	payload applicationJobSpecificationReviewPortablePayload,
) (string, map[string]any, error) {
	input, err := payload.reviewInput()
	if err != nil {
		return "", nil, err
	}
	prompt, err := BuildApplicationJobSpecificationReviewPrompt(input)
	if err != nil {
		return "", nil, err
	}
	schema, err := ApplicationJobSpecificationReviewResponseSchema(input)
	return prompt, schema, err
}

// DecodeApplicationJobSpecificationReviewResult restores the exact private
// authority carried by one portable review job before decoding its untrusted
// final response. Benchmark and transport callers therefore use the same
// semantic validator as the production review worker.
func DecodeApplicationJobSpecificationReviewResult(
	job PortableJob,
	raw string,
) (ApplicationJobSpecificationReview, error) {
	var zero ApplicationJobSpecificationReview
	if err := job.Validate(); err != nil {
		return zero, err
	}
	if job.Kind != WorkApplicationJobSpecificationReview {
		return zero, fmt.Errorf(
			"application job specification review result requires work kind %q",
			WorkApplicationJobSpecificationReview,
		)
	}
	var payload applicationJobSpecificationReviewPortablePayload
	if err := decodePortablePayload(job.Payload, &payload); err != nil {
		return zero, err
	}
	input, err := payload.reviewInput()
	if err != nil {
		return zero, err
	}
	return DecodeApplicationJobSpecificationReview(input, raw)
}

func applicationJobSpecificationBinding(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
) (string, error) {
	raw, err := exactjson.Canonical(applicationJobSpecificationRetainedAuthority{
		Authority: authority, Retained: retained,
	})
	if err != nil {
		return "", fmt.Errorf("canonicalize retained application job specification authority: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func applicationJobSpecificationCurrentFieldSHA256(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
) (string, error) {
	value, err := applicationJobSpecificationCurrentFieldValue(retained, field)
	if err != nil {
		return "", err
	}
	raw, err := exactjson.Canonical(value)
	if err != nil {
		return "", fmt.Errorf("canonicalize current application job specification field: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func applicationJobSpecificationCurrentFieldValue(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
) (any, error) {
	switch field {
	case ApplicationJobSpecificationObjectiveField:
		return retained.Objective, nil
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return append([]string(nil), retained.RequiredBehaviors...), nil
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return append([]string(nil), retained.AcceptanceCriteria...), nil
	default:
		return nil, fmt.Errorf("application job specification field %q is unsupported", field)
	}
}
