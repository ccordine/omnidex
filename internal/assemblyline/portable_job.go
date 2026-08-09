package assemblyline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	PortableJobSchemaV1       = "omnidex.portable-job.v1"
	maxPortablePayloadBytes   = 16 * 1024
	maxPortableCandidateBytes = 12 * 1024
)

type WorkKind string

const (
	WorkRequirementPartition      WorkKind = "requirement_partition"
	WorkRequirementBriefing       WorkKind = "requirement_partition_briefing"
	WorkRequirementAdvisory       WorkKind = "requirement_partition_advisory"
	WorkRequirementSynthesis      WorkKind = "requirement_partition_synthesis"
	WorkRequirementFinalAdvisory  WorkKind = "requirement_final_advisory"
	WorkRequirementFinalSynthesis WorkKind = "requirement_final_synthesis"
	WorkRepositoryRetrieval       WorkKind = "repository_retrieval"
	WorkRetrievalBriefing         WorkKind = "repository_retrieval_briefing"
	WorkRetrievalAdvisory         WorkKind = "repository_retrieval_advisory"
	WorkRetrievalSynthesis        WorkKind = "repository_retrieval_synthesis"
	WorkApplicationClassify       WorkKind = "application_classification"
	WorkApplicationIdentity       WorkKind = "application_identity"
	WorkArtifactHandling          WorkKind = "artifact_handling"
	WorkCapabilityRelation        WorkKind = "capability_relation"
	WorkSkillSelection            WorkKind = "skill_selection"
	WorkSkillProcedure            WorkKind = "skill_procedure"
	WorkFragmentGeneration        WorkKind = "fragment_generation"
	WorkFragmentCorrection        WorkKind = "fragment_correction"
	WorkResponseCorrection        WorkKind = "response_correction"
)

type PortableJob struct {
	Schema  string          `json:"schema"`
	ID      string          `json:"id"`
	Kind    WorkKind        `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

type PortableResult struct {
	JobID     string `json:"job_id"`
	Candidate string `json:"candidate"`
}

func newPortableJob(kind WorkKind, input any) (PortableJob, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return PortableJob{}, fmt.Errorf("encode portable %s input: %w", kind, err)
	}
	job := PortableJob{Schema: PortableJobSchemaV1, Kind: kind, Payload: payload}
	job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
	if err := job.Validate(); err != nil {
		return PortableJob{}, err
	}
	return job, nil
}

func (job PortableJob) Validate() error {
	if job.Schema != PortableJobSchemaV1 {
		return fmt.Errorf("portable job schema must be %q", PortableJobSchemaV1)
	}
	if !validWorkKind(job.Kind) {
		return fmt.Errorf("portable job kind %q is unsupported", job.Kind)
	}
	if len(job.Payload) == 0 || len(job.Payload) > maxPortablePayloadBytes {
		return fmt.Errorf("portable job payload must contain between 1 and %d bytes", maxPortablePayloadBytes)
	}
	expectedID := portableJobDigest(job.Schema, job.Kind, job.Payload)
	if job.ID != expectedID {
		return fmt.Errorf("portable job id does not match its immutable content")
	}

	return validatePortableJobPayload(job.Kind, job.Payload)
}

func (result PortableResult) ValidateFor(job PortableJob) error {
	if err := job.Validate(); err != nil {
		return err
	}
	if result.JobID != job.ID {
		return fmt.Errorf("portable result job id does not match the claimed job")
	}
	if strings.TrimSpace(result.Candidate) == "" {
		return fmt.Errorf("portable result candidate is empty")
	}
	if len(result.Candidate) > maxPortableCandidateBytes {
		return fmt.Errorf("portable result candidate exceeds %d bytes", maxPortableCandidateBytes)
	}
	return nil
}

func portableJobDigest(schema string, kind WorkKind, payload []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(schema))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return hex.EncodeToString(hash.Sum(nil))
}

func decodePortablePayload(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode portable job payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode portable job payload: trailing JSON value")
		}
		return fmt.Errorf("decode portable job payload: %w", err)
	}
	return nil
}

func validWorkKind(kind WorkKind) bool {
	switch kind {
	case WorkApplicationClassify, WorkApplicationIdentity, WorkRequirementPartition,
		WorkRequirementBriefing, WorkRequirementAdvisory, WorkRequirementSynthesis,
		WorkRequirementFinalAdvisory, WorkRequirementFinalSynthesis,
		WorkRepositoryRetrieval, WorkRetrievalBriefing, WorkRetrievalAdvisory, WorkRetrievalSynthesis,
		WorkArtifactHandling, WorkCapabilityRelation, WorkSkillSelection, WorkSkillProcedure,
		WorkFragmentGeneration, WorkFragmentCorrection, WorkResponseCorrection:
		return true
	default:
		return false
	}
}
