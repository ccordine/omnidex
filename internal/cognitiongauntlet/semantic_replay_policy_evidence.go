package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

type semanticPolicyEvidence struct {
	Schema              string `json:"schema"`
	EvidenceKind        string `json:"evidence_kind"`
	CallID              string `json:"call_id"`
	EvidenceID          string `json:"evidence_id"`
	ReferenceJSONSHA256 string `json:"reference_json_sha256"`
	ContentSHA256       string `json:"content_sha256"`
	Bytes               int    `json:"bytes"`
}

func (state *semanticReplayState) mapOpaquePolicyEvidence(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	value, err := decodeSemanticPolicyEvidence(record)
	if err != nil {
		return nil, err
	}
	if record.CallOrdinal < 1 || record.Phase != 32 || record.Sequence != 0 ||
		state.attemptOrdinals[record.CallOrdinal] != value.CallID {
		return nil, fmt.Errorf("semantic policy evidence lacks its exact call tuple")
	}
	draft := sourceKnowledgeDraft(
		cognitionreplay.EventEvidenceAcquired, source,
		cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgeActive,
		cognitionreplay.AuthorityTool,
	)
	draft.Knowledge.Ref = "policy-evidence://" + value.EvidenceID
	return []semanticEventDraft{draft}, nil
}

func decodeSemanticPolicyEvidence(
	record queue.CognitionSealedTraceRecord,
) (semanticPolicyEvidence, error) {
	var value semanticPolicyEvidence
	if err := decodeProductionPayload(
		record.Payload, &value, "semantic policy evidence metadata",
	); err != nil || value.Schema != "omnidex.cognition-policy-evidence-trace.v1" ||
		value.EvidenceID != record.ID || value.CallID == "" ||
		!validDigest(value.ReferenceJSONSHA256) || !validDigest(value.ContentSHA256) ||
		value.Bytes < 0 {
		return semanticPolicyEvidence{}, fmt.Errorf(
			"invalid semantic policy evidence metadata: %v", err,
		)
	}
	want := semanticPolicyEvidenceKind(record.Kind)
	if want == "" || value.EvidenceKind != want ||
		validateSemanticPolicyEvidenceBytes(value) != nil {
		return semanticPolicyEvidence{}, fmt.Errorf(
			"semantic policy evidence kind or byte contract changed",
		)
	}
	return value, nil
}

func validateSemanticPolicyEvidenceBytes(value semanticPolicyEvidence) error {
	maximum := 0
	switch value.EvidenceKind {
	case "model_response":
		maximum = cognitionpolicy.MaxModelResponseEvidenceBytes
	case "provider_generation":
		maximum = cognitionpolicy.MaxProviderGenerationEvidenceBytes
	case "provider_response_capture":
		maximum = llm.MaxExactPreparedProviderResponseBytes + 1
	default:
		return fmt.Errorf("semantic policy evidence kind is not registered")
	}
	if value.Bytes < 0 || value.Bytes > maximum ||
		(value.Bytes == 0 && value.EvidenceKind != "provider_response_capture") {
		return fmt.Errorf("semantic policy evidence byte count is outside its production bound")
	}
	return nil
}

func semanticPolicyEvidenceKind(recordKind string) string {
	switch recordKind {
	case "policy_response_evidence":
		return "model_response"
	case "policy_provider_generation_evidence":
		return "provider_generation"
	case "policy_provider_response_capture":
		return "provider_response_capture"
	default:
		return ""
	}
}
