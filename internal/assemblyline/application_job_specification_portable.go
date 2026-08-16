package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

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

type applicationJobSpecificationRepairPortablePayload struct {
	Authority           ApplicationJobSpecificationInput  `json:"authority"`
	Retained            ApplicationJobSpecification       `json:"retained"`
	Review              ApplicationJobSpecificationReview `json:"review"`
	Attempt             int                               `json:"attempt"`
	ReviewBinding       string                            `json:"review_binding"`
	ObservedValueSHA256 string                            `json:"observed_value_sha256"`
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

func newApplicationJobSpecificationRepairPortablePayload(
	input ApplicationJobSpecificationRepairInput,
) (applicationJobSpecificationRepairPortablePayload, error) {
	if err := input.validate(); err != nil {
		return applicationJobSpecificationRepairPortablePayload{}, err
	}
	payload := applicationJobSpecificationRepairPortablePayload{
		Authority: input.authority,
		Retained:  cloneApplicationJobSpecification(input.retained),
		Review: ApplicationJobSpecificationReview{
			Decision:        input.review.Decision,
			Field:           input.review.Field,
			Finding:         input.review.Finding,
			FindingEvidence: input.review.FindingEvidence,
		},
		Attempt: input.attempt, ReviewBinding: input.review.binding,
		ObservedValueSHA256: input.review.observedValueSHA256,
	}
	if err := payload.validate(); err != nil {
		return applicationJobSpecificationRepairPortablePayload{}, err
	}
	return payload, nil
}

func (payload applicationJobSpecificationRepairPortablePayload) validate() error {
	_, err := payload.repairInput()
	return err
}

func (payload applicationJobSpecificationRepairPortablePayload) repairInput() (
	ApplicationJobSpecificationRepairInput,
	error,
) {
	if err := validateApplicationJobSpecificationInput(payload.Authority); err != nil {
		return ApplicationJobSpecificationRepairInput{}, err
	}
	if err := ValidateApplicationJobSpecification(payload.Retained); err != nil {
		return ApplicationJobSpecificationRepairInput{}, err
	}
	if err := validateApplicationJobSpecificationReview(payload.Review); err != nil {
		return ApplicationJobSpecificationRepairInput{}, err
	}
	binding, err := applicationJobSpecificationBinding(payload.Authority, payload.Retained)
	if err != nil {
		return ApplicationJobSpecificationRepairInput{}, err
	}
	if err := validateApplicationJobSpecificationSHA256(
		"application job specification review binding", payload.ReviewBinding,
	); err != nil {
		return ApplicationJobSpecificationRepairInput{}, err
	}
	if payload.ReviewBinding != binding {
		return ApplicationJobSpecificationRepairInput{}, fmt.Errorf(
			"application job specification review binding does not match retained authority",
		)
	}
	if err := validateApplicationJobSpecificationSHA256(
		"application job specification observed value binding", payload.ObservedValueSHA256,
	); err != nil {
		return ApplicationJobSpecificationRepairInput{}, err
	}
	observedValueSHA256, err := applicationJobSpecificationCurrentFieldSHA256(
		payload.Retained, payload.Review.Field,
	)
	if err != nil {
		return ApplicationJobSpecificationRepairInput{}, err
	}
	if payload.ObservedValueSHA256 != observedValueSHA256 {
		return ApplicationJobSpecificationRepairInput{}, fmt.Errorf(
			"application job specification observed value binding does not match current named field",
		)
	}
	payload.Review.binding = binding
	payload.Review.observedValueSHA256 = observedValueSHA256
	return NewApplicationJobSpecificationRepairInput(
		payload.Authority, payload.Retained, payload.Review, payload.Attempt,
	)
}

func renderApplicationJobSpecificationRepairPortable(
	payload applicationJobSpecificationRepairPortablePayload,
) (string, map[string]any, error) {
	input, err := payload.repairInput()
	if err != nil {
		return "", nil, err
	}
	return renderApplicationJobSpecificationRepair(input)
}

// RestoreApplicationJobSpecificationRepairJob restores the exact private
// input and retained state from one immutable repair envelope. It exists for
// read-only production-contract replay; callers cannot construct or alter the
// private authority fields.
func RestoreApplicationJobSpecificationRepairJob(
	job PortableJob,
) (
	ApplicationJobSpecificationRepairInput,
	ApplicationJobSpecificationInput,
	ApplicationJobSpecification,
	error,
) {
	if err := job.Validate(); err != nil {
		return ApplicationJobSpecificationRepairInput{}, ApplicationJobSpecificationInput{},
			ApplicationJobSpecification{}, err
	}
	if job.Kind != WorkApplicationJobSpecificationRepair {
		return ApplicationJobSpecificationRepairInput{}, ApplicationJobSpecificationInput{},
			ApplicationJobSpecification{}, fmt.Errorf(
				"restore application job specification repair requires work kind %q",
				WorkApplicationJobSpecificationRepair,
			)
	}
	var payload applicationJobSpecificationRepairPortablePayload
	if err := decodePortablePayload(job.Payload, &payload); err != nil {
		return ApplicationJobSpecificationRepairInput{}, ApplicationJobSpecificationInput{},
			ApplicationJobSpecification{}, err
	}
	input, err := payload.repairInput()
	if err != nil {
		return ApplicationJobSpecificationRepairInput{}, ApplicationJobSpecificationInput{},
			ApplicationJobSpecification{}, err
	}
	return input, payload.Authority, cloneApplicationJobSpecification(payload.Retained), nil
}

func renderApplicationJobSpecificationRepair(
	input ApplicationJobSpecificationRepairInput,
) (string, map[string]any, error) {
	prompt, err := BuildApplicationJobSpecificationRepairPrompt(input)
	if err != nil {
		return "", nil, err
	}
	schema, err := ApplicationJobSpecificationRepairResponseSchema(input)
	return prompt, schema, err
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

func validateApplicationJobSpecificationSHA256(label string, value string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || value != strings.ToLower(value) {
		return fmt.Errorf("%s must be 64 lowercase hexadecimal characters", label)
	}
	return nil
}
