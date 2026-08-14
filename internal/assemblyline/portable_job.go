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
	PortableJobSchemaV1      = "omnidex.portable-job.v1"
	maxPortableResourceBytes = 128 * 1024
	// Raw provider output is captured and bounded before it becomes a portable
	// result. This duplicate ceiling protects test and alternate in-process
	// executors at the same coarse 16 MiB provider-response boundary.
	maxPortableRawCandidateBytes = 16 * 1024 * 1024
	// Portable byte limits are coarse resource ceilings. Station field bounds
	// and the exact provider token budget remain the semantic/context authority.
	maxPortablePayloadBytes   = maxPortableResourceBytes
	maxPortableCandidateBytes = maxPortableResourceBytes
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
	JobID      string                    `json:"job_id"`
	Candidate  string                    `json:"candidate"`
	Projection *PortableResultProjection `json:"-"`
}

type PortableResultProjectionKind string

const (
	PortableResultProjectionExactResponse      PortableResultProjectionKind = "exact_response"
	PortableResultProjectionTypeScriptFunction PortableResultProjectionKind = "typescript_function"
)

// PortableResultProjection is code-owned metadata binding the accepted leaf
// to an exact byte span in Candidate. Candidate remains the complete untrusted
// final response and is preserved separately as call evidence.
type PortableResultProjection struct {
	Kind                 PortableResultProjectionKind
	Source               string
	SourceResponseSHA256 string
	SourceSHA256         string
	StartByte            int
	EndByte              int
	RawBytes             int
	DiscardedBytes       int
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
	if len(job.Payload) == 0 {
		return fmt.Errorf("portable job payload is empty")
	}
	if len(job.Payload) > maxPortablePayloadBytes {
		return fmt.Errorf(
			"portable job payload exceeds gross resource ceiling of %d bytes",
			maxPortablePayloadBytes,
		)
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
	if len(result.Candidate) > maxPortableRawCandidateBytes {
		return fmt.Errorf(
			"portable result candidate exceeds gross resource ceiling of %d bytes",
			maxPortableRawCandidateBytes,
		)
	}
	if result.Projection != nil {
		if err := result.Projection.ValidateFor(result.Candidate); err != nil {
			return fmt.Errorf("portable result projection: %w", err)
		}
	}
	return nil
}

func NewExactPortableResultProjection(raw string) (PortableResultProjection, error) {
	projection := PortableResultProjection{
		Kind: PortableResultProjectionExactResponse, Source: raw,
		SourceResponseSHA256: portableProjectionSHA256(raw),
		SourceSHA256:         portableProjectionSHA256(raw),
		StartByte:            0, EndByte: len(raw), RawBytes: len(raw),
	}
	if err := projection.ValidateFor(raw); err != nil {
		return PortableResultProjection{}, err
	}
	return projection, nil
}

func (projection TypeScriptFunctionProjection) PortableResultProjection() (PortableResultProjection, error) {
	result := PortableResultProjection{
		Kind: PortableResultProjectionTypeScriptFunction, Source: projection.Source,
		SourceResponseSHA256: projection.RawSHA256, SourceSHA256: projection.SourceSHA256,
		StartByte: projection.StartByte, EndByte: projection.EndByte,
		RawBytes: projection.RawBytes, DiscardedBytes: projection.DiscardedBytes,
	}
	if result.EndByte-result.StartByte != len(result.Source) ||
		result.RawBytes-result.DiscardedBytes != len(result.Source) {
		return PortableResultProjection{}, fmt.Errorf("TypeScript projection metadata is internally inconsistent")
	}
	return result, nil
}

func (projection PortableResultProjection) ValidateFor(raw string) error {
	if projection.Kind != PortableResultProjectionExactResponse &&
		projection.Kind != PortableResultProjectionTypeScriptFunction {
		return fmt.Errorf("projection kind %q is not registered", projection.Kind)
	}
	if projection.RawBytes != len(raw) || projection.SourceResponseSHA256 != portableProjectionSHA256(raw) {
		return fmt.Errorf("projection raw response identity is invalid")
	}
	if projection.StartByte < 0 || projection.EndByte <= projection.StartByte ||
		projection.EndByte > len(raw) {
		return fmt.Errorf("projection source span is invalid")
	}
	if projection.Source != raw[projection.StartByte:projection.EndByte] ||
		projection.SourceSHA256 != portableProjectionSHA256(projection.Source) {
		return fmt.Errorf("projection source differs from its exact response span")
	}
	if projection.DiscardedBytes != len(raw)-len(projection.Source) {
		return fmt.Errorf("projection discarded byte count is invalid")
	}
	if projection.Kind == PortableResultProjectionExactResponse &&
		(projection.StartByte != 0 || projection.EndByte != len(raw) || projection.DiscardedBytes != 0) {
		return fmt.Errorf("exact response projection is not the complete response")
	}
	return nil
}

func portableProjectionSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
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
