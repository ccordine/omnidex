package cognitiongauntlet

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticPolicyBodyRejectsUnboundedMetadataBeforePaging(t *testing.T) {
	tests := map[string]struct {
		kind    string
		maximum int
	}{
		"model response": {
			kind: "model_response", maximum: cognitionpolicy.MaxModelResponseEvidenceBytes,
		},
		"provider generation": {
			kind: "provider_generation", maximum: cognitionpolicy.MaxProviderGenerationEvidenceBytes,
		},
		"provider response capture": {
			kind: "provider_response_capture", maximum: llm.MaxExactPreparedProviderResponseBytes + 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			metadata := semanticPolicyEvidence{
				EvidenceKind: test.kind, EvidenceID: "evidence", Bytes: test.maximum,
			}
			reader := &semanticReplayFakeEvidenceReader{
				policy: map[string]semanticReplayFakePolicyBody{},
			}
			_, _ = readSemanticPolicyBody(t.Context(), reader, "episode", metadata)
			if len(reader.policyRequests) != 1 {
				t.Fatal("exact production cap did not reach the bounded pager")
			}
			for _, bytes := range []int{test.maximum + 1, math.MaxInt} {
				reader.policyRequests = nil
				metadata.Bytes = bytes
				if _, err := readSemanticPolicyBody(
					t.Context(), reader, "episode", metadata,
				); err == nil {
					t.Fatalf("out-of-bound byte count %d was accepted", bytes)
				}
				if len(reader.policyRequests) != 0 {
					t.Fatal("out-of-bound metadata reached the pager")
				}
			}
		})
	}
}

func TestSemanticPolicyBodiesRejectAggregateAboveContainerBeforePaging(t *testing.T) {
	inventory := semanticReplayEvidenceInventory{
		policy: make(map[string]semanticPolicyEvidence),
	}
	trace := productionTrace{}
	remaining := cognitionreplay.MaxContainerBytes
	for index := 0; remaining > 0; index++ {
		bytes := cognitionpolicy.MaxModelResponseEvidenceBytes
		if bytes > remaining {
			bytes = remaining
		}
		id := fmt.Sprintf("evidence-%d", index)
		metadata := semanticPolicyEvidence{
			EvidenceKind: "model_response", EvidenceID: id, Bytes: bytes,
		}
		inventory.policy[semanticReplayEvidenceKey(metadata.EvidenceKind, id)] = metadata
		trace.Records = append(trace.Records, queue.CognitionSealedTraceRecord{
			Kind: "policy_response_evidence", ID: id,
		})
		remaining -= bytes
	}
	if err := preflightSemanticPolicyBodies(trace, inventory); err != nil {
		t.Fatalf("exact aggregate cap rejected: %v", err)
	}
	id := "evidence-over-cap"
	inventory.policy[semanticReplayEvidenceKey("model_response", id)] = semanticPolicyEvidence{
		EvidenceKind: "model_response", EvidenceID: id, Bytes: 1,
	}
	trace.Records = append(trace.Records, queue.CognitionSealedTraceRecord{
		Kind: "policy_response_evidence", ID: id,
	})
	reader := &semanticReplayFakeEvidenceReader{
		policy: map[string]semanticReplayFakePolicyBody{},
	}
	if err := readSemanticPolicyBodies(
		t.Context(), reader, "episode", trace, inventory, &semanticReplaySupplement{},
	); err == nil {
		t.Fatal("aggregate policy bodies above the container cap were accepted")
	}
	if len(reader.policyRequests) != 0 {
		t.Fatal("over-bound aggregate reached the pager")
	}
}

func TestSemanticPolicyBodyPagerIsBoundedAndBindsEveryPage(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	raw := bytes.Repeat([]byte("p"), queue.MaxCognitionPolicyEvidencePageBytes+7)
	response, err := cognitionpolicy.NewModelResponseEvidence("call-one", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	metadata, present, err := semanticPolicyMetadataForRef(
		"model_response", response.CallID, response.Ref,
	)
	if err != nil || !present {
		t.Fatalf("metadata present=%t err=%v", present, err)
	}
	reader := &semanticReplayFakeEvidenceReader{policy: map[string]semanticReplayFakePolicyBody{
		semanticReplayEvidenceKey(metadata.EvidenceKind, metadata.EvidenceID): {
			metadata: metadata, raw: raw,
		},
	}}
	got, err := readSemanticPolicyBody(t.Context(), reader, episode, metadata)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("body bytes=%d err=%v", len(got), err)
	}
	if len(reader.policyRequests) != 2 || reader.policyRequests[0].offset != 0 ||
		reader.policyRequests[1].offset != queue.MaxCognitionPolicyEvidencePageBytes ||
		reader.policyRequests[0].limit != queue.MaxCognitionPolicyEvidencePageBytes {
		t.Fatalf("requests=%+v", reader.policyRequests)
	}
}

func TestSemanticPolicyBodyPagerRejectsEveryAuthorityDrift(t *testing.T) {
	response, err := cognitionpolicy.NewModelResponseEvidence("call-one", "model response")
	if err != nil {
		t.Fatal(err)
	}
	metadata, _, err := semanticPolicyMetadataForRef(
		"model_response", response.CallID, response.Ref,
	)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*queue.CognitionPolicyEvidencePage){
		"call ID":     func(page *queue.CognitionPolicyEvidencePage) { page.CallID = "call-two" },
		"evidence ID": func(page *queue.CognitionPolicyEvidencePage) { page.EvidenceID += "-changed" },
		"SHA":         func(page *queue.CognitionPolicyEvidencePage) { page.SHA256 = strings.Repeat("f", 64) },
		"bytes":       func(page *queue.CognitionPolicyEvidencePage) { page.TotalBytes++ },
		"offset":      func(page *queue.CognitionPolicyEvidencePage) { page.Offset++ },
		"next":        func(page *queue.CognitionPolicyEvidencePage) { page.NextOffset-- },
		"truncate": func(page *queue.CognitionPolicyEvidencePage) {
			page.Content = page.Content[:len(page.Content)-1]
			page.NextOffset--
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			reader := &semanticReplayFakeEvidenceReader{
				policy: map[string]semanticReplayFakePolicyBody{
					semanticReplayEvidenceKey(metadata.EvidenceKind, metadata.EvidenceID): {
						metadata: metadata, raw: response.Content,
					},
				},
				policyMutate: func(_ string, page *queue.CognitionPolicyEvidencePage) { mutate(page) },
			}
			if _, err := readSemanticPolicyBody(
				t.Context(), reader, cognition.EpisodeID("episode-"+strings.Repeat("a", 64)), metadata,
			); err == nil {
				t.Fatal("changed policy page was accepted")
			}
		})
	}
}

func TestSemanticPolicyMetadataBindsCanonicalReferenceAndChunkedBody(t *testing.T) {
	raw := bytes.Repeat([]byte("x"), cognitionreplay.MaxDirectBlobBytes+1)
	response, err := cognitionpolicy.NewModelResponseEvidence("call-large", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	metadata, present, err := semanticPolicyMetadataForRef(
		"model_response", response.CallID, response.Ref,
	)
	if err != nil || !present {
		t.Fatal(err)
	}
	refJSON, err := exactjson.Canonical(response.Ref)
	if err != nil || metadata.ReferenceJSONSHA256 != digestExactBytes(refJSON) {
		t.Fatal("canonical policy evidence reference was not bound")
	}
	content, chunked, blobs, err := semanticReplayPolicyBodyContent(
		metadata.EvidenceKind, metadata, raw,
	)
	if err != nil || content.Storage != cognitionreplay.ProjectionContentChunked ||
		len(chunked) != 1 || len(blobs) < 2 {
		t.Fatalf("content=%+v chunked=%d blobs=%d err=%v", content, len(chunked), len(blobs), err)
	}
}

func TestSemanticEmptyPolicyCaptureRequiresOneExplicitEmptyPage(t *testing.T) {
	capture, err := cognitionpolicy.NewProviderResponseCaptureEvidence("call-empty", nil)
	if err != nil {
		t.Fatal(err)
	}
	metadata, present, err := semanticPolicyMetadataForRef(
		"provider_response_capture", capture.CallID, capture.Ref,
	)
	if err != nil || !present {
		t.Fatal(err)
	}
	reader := &semanticReplayFakeEvidenceReader{policy: map[string]semanticReplayFakePolicyBody{
		semanticReplayEvidenceKey(metadata.EvidenceKind, metadata.EvidenceID): {
			metadata: metadata, raw: []byte{},
		},
	}}
	got, err := readSemanticPolicyBody(
		t.Context(), reader, cognition.EpisodeID("episode-"+strings.Repeat("a", 64)), metadata,
	)
	if err != nil || got == nil || len(got) != 0 || len(reader.policyRequests) != 1 {
		t.Fatalf("empty capture=%v requests=%d err=%v", got, len(reader.policyRequests), err)
	}
	metadata.EvidenceKind = "model_response"
	if _, err := readSemanticPolicyBody(
		t.Context(), reader, cognition.EpisodeID("episode-"+strings.Repeat("a", 64)), metadata,
	); err == nil {
		t.Fatal("empty model response evidence was accepted")
	}
}
