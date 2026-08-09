package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const RequirementFinalAdvisoryProtocolV2 = "omnidex.requirement-final-advisory.v2"

type RequirementFinalSubject struct {
	Protocol              string                       `json:"protocol"`
	SourceText            string                       `json:"source_text"`
	DirectCandidate       RequirementPartitionDecision `json:"direct_candidate"`
	DirectCandidateSHA256 string                       `json:"direct_candidate_sha256"`
	SubjectSHA256         string                       `json:"subject_sha256"`
}

type RequirementFinalAdvisoryInput struct {
	Subject RequirementFinalSubject `json:"subject"`
}

type RequirementFinalSynthesisInput struct {
	Subject            RequirementFinalSubject `json:"subject"`
	AdvisoryJobID      string                  `json:"advisory_job_id"`
	AdvisoryMemo       string                  `json:"advisory_memo"`
	AdvisoryMemoSHA256 string                  `json:"advisory_memo_sha256"`
}

func NewRequirementFinalSubject(
	source string,
	candidate RequirementPartitionDecision,
) (RequirementFinalSubject, error) {
	subject := RequirementFinalSubject{
		Protocol: RequirementFinalAdvisoryProtocolV2, SourceText: source,
		DirectCandidate: RequirementPartitionDecision{
			Schema: candidate.Schema, FeatureQuotes: append([]string(nil), candidate.FeatureQuotes...),
		},
	}
	if err := ValidateCompleteRequirementPartition(source, subject.DirectCandidate); err != nil {
		return RequirementFinalSubject{}, fmt.Errorf("requirement final direct candidate: %w", err)
	}
	var err error
	subject.DirectCandidateSHA256, err = requirementFinalCandidateDigest(subject.DirectCandidate)
	if err != nil {
		return RequirementFinalSubject{}, err
	}
	subject.SubjectSHA256 = requirementFinalSubjectDigest(subject)
	return subject, nil
}

func NewRequirementFinalAdvisoryJob(subject RequirementFinalSubject) (PortableJob, error) {
	input := RequirementFinalAdvisoryInput{Subject: subject}
	return newValidatedPortableJob(WorkRequirementFinalAdvisory, input, input.validate)
}

func NewRequirementFinalSynthesisJob(
	subject RequirementFinalSubject,
	advisoryJob PortableJob,
	memo string,
) (PortableJob, error) {
	if advisoryJob.Kind != WorkRequirementFinalAdvisory {
		return PortableJob{}, fmt.Errorf("requirement final synthesis requires a final advisory job")
	}
	input := RequirementFinalSynthesisInput{
		Subject: subject, AdvisoryJobID: advisoryJob.ID,
		AdvisoryMemo: memo, AdvisoryMemoSHA256: requirementFinalTextDigest(memo),
	}
	return newValidatedPortableJob(WorkRequirementFinalSynthesis, input, input.validate)
}

func ValidateCompleteRequirementPartition(source string, candidate RequirementPartitionDecision) error {
	input := RequirementPartitionInput{SourceText: source, Mode: RequirementExtractFeatures}
	if err := input.validate(); err != nil {
		return err
	}
	if err := candidate.ValidateFor(input); err != nil {
		return err
	}
	if len(candidate.FeatureQuotes) == 0 {
		return fmt.Errorf("complete requirement partition requires at least one grounded feature quote")
	}
	if _, err := BuildRequirementResidual(source, candidate.FeatureQuotes); err != nil {
		return fmt.Errorf("complete requirement partition residual: %w", err)
	}
	if _, err := BuildRequirementGraph(source, candidate.FeatureQuotes); err != nil {
		return fmt.Errorf("complete requirement partition graph: %w", err)
	}
	return nil
}

func (input RequirementFinalAdvisoryInput) validate() error {
	return validateRequirementFinalSubject(input.Subject)
}

func (input RequirementFinalSynthesisInput) validate() error {
	if err := validateRequirementFinalSubject(input.Subject); err != nil {
		return err
	}
	if strings.TrimSpace(input.AdvisoryMemo) == "" {
		return fmt.Errorf("requirement final synthesis requires a non-empty advisory memo")
	}
	if len(input.AdvisoryMemo) > maxRequirementAdvisoryMemoBytes {
		return fmt.Errorf("requirement final advisory memo exceeds %d bytes", maxRequirementAdvisoryMemoBytes)
	}
	if input.AdvisoryMemoSHA256 != requirementFinalTextDigest(input.AdvisoryMemo) {
		return fmt.Errorf("requirement final advisory memo hash does not match its content")
	}
	advisoryJob, err := NewRequirementFinalAdvisoryJob(input.Subject)
	if err != nil {
		return err
	}
	if input.AdvisoryJobID != advisoryJob.ID {
		return fmt.Errorf("requirement final synthesis advisory job does not match its subject")
	}
	return nil
}

func validateRequirementFinalSubject(subject RequirementFinalSubject) error {
	if subject.Protocol != RequirementFinalAdvisoryProtocolV2 {
		return fmt.Errorf("requirement final advisory protocol must be %q", RequirementFinalAdvisoryProtocolV2)
	}
	if err := ValidateCompleteRequirementPartition(subject.SourceText, subject.DirectCandidate); err != nil {
		return fmt.Errorf("requirement final advisory subject: %w", err)
	}
	candidateHash, err := requirementFinalCandidateDigest(subject.DirectCandidate)
	if err != nil {
		return err
	}
	if subject.DirectCandidateSHA256 != candidateHash {
		return fmt.Errorf("requirement final direct candidate hash does not match its content")
	}
	if subject.SubjectSHA256 != requirementFinalSubjectDigest(subject) {
		return fmt.Errorf("requirement final advisory subject hash does not match its content")
	}
	return nil
}

func requirementFinalCandidateDigest(candidate RequirementPartitionDecision) (string, error) {
	raw, err := json.Marshal(candidate)
	if err != nil {
		return "", fmt.Errorf("encode requirement final direct candidate: %w", err)
	}
	return requirementFinalTextDigest(string(raw)), nil
}

func requirementFinalSubjectDigest(subject RequirementFinalSubject) string {
	return requirementFinalTextDigest(strings.Join([]string{
		subject.Protocol, subject.SourceText, subject.DirectCandidateSHA256,
	}, "\x00"))
}

func requirementFinalTextDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
