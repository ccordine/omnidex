package cognitionreplay

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/llm"
)

func reconstructProviderIdentityEvidence(
	replay ProviderIdentityEvidenceReplay,
	sources map[uint64]SourceRecord,
	chunked []ChunkedBlobBinding,
	blobs map[string]Blob,
) (llm.ProviderIdentityEvidence, map[uint64]struct{}, error) {
	bindings := make(map[string]ChunkedBlobBinding, len(chunked))
	for _, binding := range chunked {
		bindings[binding.Manifest.SHA256] = binding
	}
	bodySources := make(map[uint64]struct{})
	operations := make([]llm.ProviderIdentityOperationEvidence, len(replay.Operations))
	for index, operation := range replay.Operations {
		request, err := resolveProviderIdentityBody(
			operation.Request, SourceProviderIdentityRequestBody,
			providerBodySourceID(replay.Ref.ID, index, "request"), int64(index+1), 1,
			sources, bindings, blobs, bodySources,
		)
		if err != nil {
			return llm.ProviderIdentityEvidence{}, nil, err
		}
		response, err := resolveProviderIdentityBody(
			operation.Response, SourceProviderIdentityResponseBody,
			providerBodySourceID(replay.Ref.ID, index, "response"), int64(index+1), 2,
			sources, bindings, blobs, bodySources,
		)
		if err != nil {
			return llm.ProviderIdentityEvidence{}, nil, err
		}
		operations[index] = llm.ProviderIdentityOperationEvidence{
			Operation: operation.Operation, Method: operation.Method, Endpoint: operation.Endpoint,
			RequestDisposition: operation.RequestDisposition,
			RequestSHA256:      operation.Request.SHA256, RequestBytes: operation.Request.ByteCount,
			Request: request, HTTPStatus: operation.HTTPStatus, Disposition: operation.Disposition,
			ResponseComplete: operation.ResponseComplete, ContentEncoding: operation.ContentEncoding,
			ResponseSHA256: operation.Response.SHA256, ResponseBytes: operation.Response.ByteCount,
			ResponseCapture: response,
		}
	}
	evidence := llm.ProviderIdentityEvidence{
		Schema: llm.ProviderIdentityEvidenceSchemaV1, Ref: replay.Ref, Operations: operations,
	}
	if err := evidence.Validate(); err != nil {
		return llm.ProviderIdentityEvidence{}, nil,
			fmt.Errorf("replay provider identity evidence is invalid: %w", err)
	}
	return evidence, bodySources, nil
}

func resolveProviderIdentityBody(
	binding ProviderIdentityBodyBinding,
	wantKind string,
	wantID string,
	wantCall int64,
	wantPhase int,
	sources map[uint64]SourceRecord,
	chunked map[string]ChunkedBlobBinding,
	blobs map[string]Blob,
	used map[uint64]struct{},
) ([]byte, error) {
	if err := validateProviderIdentityBodyBinding(binding); err != nil {
		return nil, err
	}
	if binding.Storage == EvidenceBodyEmpty {
		return []byte{}, nil
	}
	source, exists := sources[binding.Source.Ordinal]
	if !exists || source.Ref() != *binding.Source || source.Kind != wantKind || source.ID != wantID ||
		source.CallOrdinal != wantCall || source.Phase != wantPhase || source.Sequence != 1 {
		return nil, fmt.Errorf("replay provider identity body source changed")
	}
	if _, duplicate := used[source.Ordinal]; duplicate {
		return nil, fmt.Errorf("replay provider identity body source is reused")
	}
	used[source.Ordinal] = struct{}{}
	if chunkBinding, exists := chunked[source.Payload.SHA256]; exists {
		if binding.ByteCount <= maxBlobBytes || source.Payload.MediaType != "application/json" {
			return nil, fmt.Errorf("replay provider body used a noncanonical chunk boundary")
		}
		raw, _, err := verifyChunkedBlobBinding(
			chunkBinding, blobs, ChunkedBlobPublicAgentKnowledge,
		)
		if err != nil {
			return nil, err
		}
		manifestBlob := blobs[chunkBinding.Manifest.SHA256]
		var manifest ChunkedBlobManifest
		if err := decodeCanonical(
			manifestBlob.Data, &manifest, "provider body chunk manifest",
		); err != nil || manifest.ID != wantID {
			return nil, fmt.Errorf("replay provider body chunk identity changed: %v", err)
		}
		if len(raw) != binding.ByteCount || digestBytes(raw) != binding.SHA256 {
			return nil, fmt.Errorf("chunked replay provider body changed")
		}
		return raw, nil
	}
	if binding.ByteCount > maxBlobBytes || source.Payload.MediaType != "application/octet-stream" ||
		source.Payload.ByteCount != binding.ByteCount || source.Payload.SHA256 != binding.SHA256 {
		return nil, fmt.Errorf("direct replay provider body changed")
	}
	raw, err := exactSourcePayload(source, blobs)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func validatePreEpisodeSources(
	values []SourceRecord,
	terminal PreEpisodeBrainBootstrapFailureTerminal,
	bodySources map[uint64]struct{},
) error {
	wantFixed := []SourceRef{
		terminal.PublicRunAuthority, terminal.FailureAuthority,
		terminal.FailureReceipt, terminal.IdentityEvidence,
	}
	if len(values) < len(wantFixed) {
		return fmt.Errorf("pre-episode replay fixed sources are incomplete")
	}
	for index, ref := range wantFixed {
		if ref.Ordinal != uint64(index+1) || values[index].Ref() != ref ||
			values[index].CallOrdinal != 0 || values[index].Phase != 1 ||
			values[index].Sequence != int64(index+1) {
			return fmt.Errorf("pre-episode replay fixed source order changed")
		}
	}
	if len(values) != len(wantFixed)+len(bodySources) {
		return fmt.Errorf("pre-episode replay source cardinality changed")
	}
	for index := len(wantFixed); index < len(values); index++ {
		source := values[index]
		if _, exists := bodySources[source.Ordinal]; !exists ||
			(source.Kind != SourceProviderIdentityRequestBody &&
				source.Kind != SourceProviderIdentityResponseBody) {
			return fmt.Errorf("pre-episode replay contains an unrelated source")
		}
	}
	return nil
}

func sortedSourceRefs(values ...SourceRef) []SourceRef {
	result := append([]SourceRef(nil), values...)
	sort.Slice(result, func(left, right int) bool { return result[left].Ordinal < result[right].Ordinal })
	return result
}
