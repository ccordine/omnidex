package cognitiongauntlet

import (
	"context"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func readSemanticProviderIdentities(
	ctx context.Context,
	reader ProductionSemanticReplayReader,
	episodeID cognition.EpisodeID,
	inventory semanticReplayEvidenceInventory,
	supplement *semanticReplaySupplement,
) (map[string]llm.ProviderIdentityEvidence, error) {
	ids := make([]string, 0, len(inventory.identities))
	for id := range inventory.identities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make(map[string]llm.ProviderIdentityEvidence, len(ids))
	for _, id := range ids {
		evidence, err := readSemanticProviderIdentity(
			ctx, reader, episodeID, inventory.identities[id], supplement,
		)
		if err != nil {
			return nil, err
		}
		result[id] = evidence
	}
	return result, nil
}

func readSemanticProviderIdentity(
	ctx context.Context,
	reader ProductionSemanticReplayReader,
	episodeID cognition.EpisodeID,
	want llm.ProviderIdentityEvidenceRef,
	supplement *semanticReplaySupplement,
) (llm.ProviderIdentityEvidence, error) {
	manifest, err := reader.ReadCognitionProviderIdentityEvidenceManifest(
		ctx, episodeID, want.ID,
	)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, fmt.Errorf(
			"read semantic provider identity manifest %q: %w", want.ID, err,
		)
	}
	if manifest.Ref != want || len(manifest.Operations) != 5 {
		return llm.ProviderIdentityEvidence{}, fmt.Errorf("semantic provider identity manifest changed")
	}
	if err := validateSemanticIdentityMetadata(manifest); err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	operations := make([]llm.ProviderIdentityOperationEvidence, 5)
	for index, metadata := range manifest.Operations {
		if metadata.Index != index {
			return llm.ProviderIdentityEvidence{}, fmt.Errorf("semantic provider identity operations were reordered")
		}
		request, err := readSemanticProviderIdentityBody(
			ctx, reader, episodeID, want, metadata,
			queue.CognitionProviderIdentityRequestBody,
		)
		if err != nil {
			return llm.ProviderIdentityEvidence{}, err
		}
		response, err := readSemanticProviderIdentityBody(
			ctx, reader, episodeID, want, metadata,
			queue.CognitionProviderIdentityResponseBody,
		)
		if err != nil {
			return llm.ProviderIdentityEvidence{}, err
		}
		operations[index] = semanticProviderIdentityOperation(metadata, request, response)
		if err := addSemanticProviderIdentityBody(
			supplement, want.ID, index, semanticSidecarProviderIdentityRequest, request,
		); err != nil {
			return llm.ProviderIdentityEvidence{}, err
		}
		if err := addSemanticProviderIdentityBody(
			supplement, want.ID, index, semanticSidecarProviderIdentityResponse, response,
		); err != nil {
			return llm.ProviderIdentityEvidence{}, err
		}
	}
	evidence := llm.ProviderIdentityEvidence{
		Schema: llm.ProviderIdentityEvidenceSchemaV1, Ref: want, Operations: operations,
	}
	if err := evidence.Validate(); err != nil {
		return llm.ProviderIdentityEvidence{}, fmt.Errorf(
			"semantic provider identity evidence %q changed: %w", want.ID, err,
		)
	}
	manifestEvidence := evidence.Clone()
	for index := range manifestEvidence.Operations {
		manifestEvidence.Operations[index].Request = nil
		manifestEvidence.Operations[index].ResponseCapture = nil
	}
	content, chunked, blobs, err := semanticReplayProjectionContent(
		"provider-identity-manifest-"+want.ID, manifestEvidence,
	)
	if err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	if err := supplement.add(
		semanticSidecarProviderIdentityManifest, want.ID, content, chunked, blobs,
	); err != nil {
		return llm.ProviderIdentityEvidence{}, err
	}
	return evidence.Clone(), nil
}

func validateSemanticIdentityMetadata(
	manifest queue.CognitionProviderIdentityEvidenceManifest,
) error {
	want := []struct {
		operation llm.ProviderIdentityOperation
		method    string
		endpoint  string
	}{
		{llm.ProviderIdentityVersion, "GET", "/api/version"},
		{llm.ProviderIdentityInstalled, "GET", "/api/tags"},
		{llm.ProviderIdentityTokenizer, "POST", "/api/show"},
		{llm.ProviderIdentityPreload, "POST", "/api/generate"},
		{llm.ProviderIdentityRunner, "GET", "/api/ps"},
	}
	total := 0
	for index, metadata := range manifest.Operations {
		if metadata.Index != index || metadata.RequestBytes < 0 ||
			metadata.RequestBytes > llm.MaxProviderIdentityComponentBytes ||
			metadata.ResponseBytes < 0 ||
			metadata.ResponseBytes > llm.MaxProviderIdentityComponentBytes+1 ||
			!validDigest(metadata.RequestSHA256) || !validDigest(metadata.ResponseSHA256) ||
			metadata.Operation != want[index].operation || metadata.Method != want[index].method ||
			metadata.Endpoint != want[index].endpoint || metadata.RequestDisposition.Validate() != nil ||
			!semanticIdentityDisposition(metadata.Disposition) ||
			(metadata.Disposition != llm.ProviderIdentityNotDispatched &&
				metadata.Disposition != llm.ProviderIdentityTransport &&
				metadata.ContentEncoding.Validate() != nil) {
			return fmt.Errorf("semantic provider identity operation metadata is outside its byte authority")
		}
		total += metadata.RequestBytes + metadata.ResponseBytes
		if total > llm.MaxProviderIdentityEvidenceBytes || total > manifest.Ref.Bytes {
			return fmt.Errorf("semantic provider identity manifest exceeds its exact cumulative bytes")
		}
	}
	if total != manifest.Ref.Bytes {
		return fmt.Errorf("semantic provider identity manifest byte total changed")
	}
	return nil
}

func semanticIdentityDisposition(value llm.ProviderIdentityOperationDisposition) bool {
	switch value {
	case llm.ProviderIdentityNotDispatched, llm.ProviderIdentitySucceeded,
		llm.ProviderIdentityTransport, llm.ProviderIdentityHTTPError,
		llm.ProviderIdentityBodyLimit, llm.ProviderIdentityBodyReadError,
		llm.ProviderIdentityInvalidJSON:
		return true
	default:
		return false
	}
}

func readSemanticProviderIdentityBody(
	ctx context.Context,
	reader ProductionSemanticReplayReader,
	episodeID cognition.EpisodeID,
	ref llm.ProviderIdentityEvidenceRef,
	metadata queue.CognitionProviderIdentityOperationMetadata,
	kind queue.CognitionProviderIdentityBodyKind,
) ([]byte, error) {
	wantSHA, wantBytes := metadata.RequestSHA256, metadata.RequestBytes
	if kind == queue.CognitionProviderIdentityResponseBody {
		wantSHA, wantBytes = metadata.ResponseSHA256, metadata.ResponseBytes
	}
	raw := make([]byte, 0, wantBytes)
	offset := 0
	for {
		page, err := reader.ReadCognitionProviderIdentityEvidenceBody(
			ctx, episodeID, ref.ID, metadata.Index, kind, offset,
			queue.MaxCognitionPolicyEvidencePageBytes,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"read semantic provider identity %s %q/%d at %d: %w",
				kind, ref.ID, metadata.Index, offset, err,
			)
		}
		if page.Ref != ref || page.OperationIndex != metadata.Index || page.Kind != kind ||
			page.SHA256 != wantSHA || page.TotalBytes != wantBytes || page.Offset != offset ||
			page.NextOffset != offset+len(page.Content) ||
			len(page.Content) > queue.MaxCognitionPolicyEvidencePageBytes ||
			page.NextOffset > wantBytes {
			return nil, fmt.Errorf("semantic provider identity body page authority changed")
		}
		raw = append(raw, page.Content...)
		if page.NextOffset == wantBytes {
			break
		}
		if len(page.Content) == 0 || page.NextOffset <= offset {
			return nil, fmt.Errorf("semantic provider identity body pager made no progress")
		}
		offset = page.NextOffset
	}
	if len(raw) != wantBytes || digestExactBytes(raw) != wantSHA {
		return nil, fmt.Errorf("semantic provider identity body changed across its pages")
	}
	return raw, nil
}

func semanticProviderIdentityOperation(
	value queue.CognitionProviderIdentityOperationMetadata,
	request []byte,
	response []byte,
) llm.ProviderIdentityOperationEvidence {
	return llm.ProviderIdentityOperationEvidence{
		Operation: value.Operation, Method: value.Method, Endpoint: value.Endpoint,
		RequestDisposition: value.RequestDisposition,
		RequestSHA256:      value.RequestSHA256, RequestBytes: value.RequestBytes,
		Request: request, HTTPStatus: value.HTTPStatus, Disposition: value.Disposition,
		ResponseComplete: value.ResponseComplete, ContentEncoding: value.ContentEncoding,
		ResponseSHA256: value.ResponseSHA256, ResponseBytes: value.ResponseBytes,
		ResponseCapture: response,
	}
}

func addSemanticProviderIdentityBody(
	supplement *semanticReplaySupplement,
	evidenceID string,
	operation int,
	kind string,
	raw []byte,
) error {
	content, chunked, blobs, err := semanticReplayContentForBytes(
		"provider-identity-body-"+semanticIdentityBodyID(evidenceID, operation)+"-"+kind,
		raw,
	)
	if err != nil {
		return err
	}
	return supplement.add(kind, semanticIdentityBodyID(evidenceID, operation), content, chunked, blobs)
}
