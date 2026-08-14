package assemblyline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	PortableJobSchemaV1       = "omnidex.portable-job.v1"
	maxPortablePayloadBytes   = 16 * 1024
	maxPortableCandidateBytes = 12 * 1024
)

type WorkKind string

const (
	WorkApplicationRequirements           WorkKind = "application_requirements"
	WorkApplicationJobSpecification       WorkKind = "application_job_specification"
	WorkApplicationJobSpecificationReview WorkKind = "application_job_specification_review"
	WorkApplicationJobSpecificationRepair WorkKind = "application_job_specification_repair"
	WorkRepositoryRequirements            WorkKind = "repository_requirements"
	WorkRepositorySearchTerm              WorkKind = "repository_search_term"
	WorkRepositoryChangeSurface           WorkKind = "repository_change_surface"
	WorkRepositoryEvidenceRelevance       WorkKind = "repository_evidence_relevance"
	WorkRepositoryGroundedReview          WorkKind = "repository_grounded_review"
	WorkRepositoryGroundedCorrection      WorkKind = "repository_grounded_correction"
	WorkConversationContextSelection      WorkKind = "conversation_context_selection"
	WorkMemoryContextSelection            WorkKind = "memory_context_selection"
	WorkConversationObjectiveKind         WorkKind = "conversation_objective_kind"
	WorkConversationResponse              WorkKind = "conversation_response"
	WorkGroundedAnswer                    WorkKind = "grounded_answer"
	WorkWebSearchTerms                    WorkKind = "web_search_terms"
	WorkWebRelevance                      WorkKind = "web_relevance"
	WorkWebGroundedSynthesis              WorkKind = "web_grounded_synthesis"
	WorkWebGroundedSynthesisCorrection    WorkKind = "web_grounded_synthesis_correction"
	WorkWebClaimEvidenceReview            WorkKind = "web_claim_evidence_review"
	WorkApplicationClassify               WorkKind = "application_classification"
	WorkArtifactHandling                  WorkKind = "artifact_handling"
	WorkKnownArtifactTruth                WorkKind = "known_artifact_truth"
	WorkDeclarationArtifactBoundary       WorkKind = "declaration_artifact_boundary"
	WorkArtifactCandidateSelection        WorkKind = "artifact_candidate_selection"
	WorkCapabilityRelation                WorkKind = "capability_relation"
	WorkSkillSelection                    WorkKind = "skill_selection"
	WorkFragmentGeneration                WorkKind = "fragment_generation"
	WorkFragmentModification              WorkKind = "fragment_modification"
	WorkFragmentCorrection                WorkKind = "fragment_correction"
	WorkResponseCorrection                WorkKind = "response_correction"
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
	if !utf8.Valid(payload) {
		return fmt.Errorf("decode portable job payload: invalid UTF-8")
	}
	if err := exactjson.ValidateObject(payload, target, "portable job payload"); err != nil {
		return fmt.Errorf("decode portable job payload: %w", err)
	}
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
	case WorkApplicationClassify, WorkApplicationRequirements,
		WorkApplicationJobSpecification, WorkApplicationJobSpecificationReview,
		WorkApplicationJobSpecificationRepair, WorkRepositoryRequirements,
		WorkRepositorySearchTerm, WorkRepositoryChangeSurface, WorkRepositoryEvidenceRelevance,
		WorkRepositoryGroundedReview, WorkRepositoryGroundedCorrection,
		WorkConversationContextSelection, WorkMemoryContextSelection,
		WorkConversationObjectiveKind, WorkConversationResponse, WorkGroundedAnswer,
		WorkWebSearchTerms, WorkWebRelevance, WorkWebGroundedSynthesis,
		WorkWebGroundedSynthesisCorrection, WorkWebClaimEvidenceReview,
		WorkArtifactHandling, WorkKnownArtifactTruth,
		WorkDeclarationArtifactBoundary, WorkArtifactCandidateSelection,
		WorkCapabilityRelation, WorkSkillSelection,
		WorkFragmentGeneration, WorkFragmentModification, WorkFragmentCorrection, WorkResponseCorrection:
		return true
	default:
		return false
	}
}

// AllWorkKinds returns the closed PortableJob kind registry. Callers may use
// it to prove exhaustive station mappings without inventing string routing.
func AllWorkKinds() []WorkKind {
	return []WorkKind{
		WorkApplicationClassify, WorkApplicationRequirements,
		WorkApplicationJobSpecification, WorkApplicationJobSpecificationReview,
		WorkApplicationJobSpecificationRepair, WorkRepositoryRequirements,
		WorkRepositorySearchTerm, WorkRepositoryChangeSurface, WorkRepositoryEvidenceRelevance,
		WorkRepositoryGroundedReview, WorkRepositoryGroundedCorrection,
		WorkConversationContextSelection, WorkMemoryContextSelection,
		WorkConversationObjectiveKind, WorkConversationResponse, WorkGroundedAnswer,
		WorkWebSearchTerms, WorkWebRelevance, WorkWebGroundedSynthesis,
		WorkWebGroundedSynthesisCorrection, WorkWebClaimEvidenceReview,
		WorkArtifactHandling, WorkKnownArtifactTruth,
		WorkDeclarationArtifactBoundary, WorkArtifactCandidateSelection,
		WorkCapabilityRelation, WorkSkillSelection,
		WorkFragmentGeneration, WorkFragmentModification, WorkFragmentCorrection, WorkResponseCorrection,
	}
}
