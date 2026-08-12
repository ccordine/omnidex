package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

type semanticReplayFakeEvidenceReader struct {
	policy          map[string]semanticReplayFakePolicyBody
	identities      map[string]llm.ProviderIdentityEvidence
	policyMutate    func(string, *queue.CognitionPolicyEvidencePage)
	identityMutate  func(*queue.CognitionProviderIdentityEvidenceBodyPage)
	manifestMutate  func(*queue.CognitionProviderIdentityEvidenceManifest)
	policyRequests  []semanticReplayPageRequest
	identityRequest []semanticReplayIdentityPageRequest
}

type semanticReplayFakePolicyBody struct {
	metadata semanticPolicyEvidence
	raw      []byte
}

type semanticReplayPageRequest struct {
	kind, id      string
	offset, limit int
}

type semanticReplayIdentityPageRequest struct {
	id            string
	operation     int
	kind          queue.CognitionProviderIdentityBodyKind
	offset, limit int
}

func (*semanticReplayFakeEvidenceReader) ReadCognitionSealedTrace(
	context.Context,
	cognition.EpisodeID,
	queue.CognitionTracePageRequest,
) (queue.CognitionSealedTracePage, error) {
	return queue.CognitionSealedTracePage{}, fmt.Errorf("fake sealed trace is unavailable")
}

func (reader *semanticReplayFakeEvidenceReader) ReadCognitionModelResponseEvidence(
	ctx context.Context, episode cognition.EpisodeID, id string, offset, limit int,
) (queue.CognitionPolicyEvidencePage, error) {
	return reader.readPolicy(ctx, episode, "model_response", id, offset, limit)
}

func (reader *semanticReplayFakeEvidenceReader) ReadCognitionProviderGenerationEvidence(
	ctx context.Context, episode cognition.EpisodeID, id string, offset, limit int,
) (queue.CognitionPolicyEvidencePage, error) {
	return reader.readPolicy(ctx, episode, "provider_generation", id, offset, limit)
}

func (reader *semanticReplayFakeEvidenceReader) ReadCognitionProviderResponseCapture(
	ctx context.Context, episode cognition.EpisodeID, id string, offset, limit int,
) (queue.CognitionPolicyEvidencePage, error) {
	return reader.readPolicy(ctx, episode, "provider_response_capture", id, offset, limit)
}

func (reader *semanticReplayFakeEvidenceReader) readPolicy(
	_ context.Context,
	_ cognition.EpisodeID,
	kind, id string,
	offset, limit int,
) (queue.CognitionPolicyEvidencePage, error) {
	reader.policyRequests = append(reader.policyRequests, semanticReplayPageRequest{
		kind: kind, id: id, offset: offset, limit: limit,
	})
	value, exists := reader.policy[semanticReplayEvidenceKey(kind, id)]
	if !exists || offset < 0 || offset > len(value.raw) || limit < 1 {
		return queue.CognitionPolicyEvidencePage{}, fmt.Errorf("unexpected fake policy page")
	}
	end := offset + limit
	if end > len(value.raw) {
		end = len(value.raw)
	}
	page := queue.CognitionPolicyEvidencePage{
		CallID: value.metadata.CallID, EvidenceID: value.metadata.EvidenceID,
		SHA256: value.metadata.ContentSHA256, TotalBytes: value.metadata.Bytes,
		Offset: offset, NextOffset: end, Content: append([]byte{}, value.raw[offset:end]...),
	}
	if reader.policyMutate != nil {
		reader.policyMutate(kind, &page)
	}
	return page, nil
}

func (reader *semanticReplayFakeEvidenceReader) ReadCognitionProviderIdentityEvidenceManifest(
	_ context.Context,
	_ cognition.EpisodeID,
	id string,
) (queue.CognitionProviderIdentityEvidenceManifest, error) {
	evidence, exists := reader.identities[id]
	if !exists {
		return queue.CognitionProviderIdentityEvidenceManifest{}, fmt.Errorf("unexpected fake identity manifest")
	}
	manifest := queue.CognitionProviderIdentityEvidenceManifest{Ref: evidence.Ref}
	for index, operation := range evidence.Operations {
		manifest.Operations = append(manifest.Operations, queue.CognitionProviderIdentityOperationMetadata{
			Index: index, Operation: operation.Operation, Method: operation.Method,
			Endpoint: operation.Endpoint, RequestDisposition: operation.RequestDisposition,
			RequestSHA256: operation.RequestSHA256, RequestBytes: operation.RequestBytes,
			HTTPStatus: operation.HTTPStatus, Disposition: operation.Disposition,
			ResponseComplete: operation.ResponseComplete, ContentEncoding: operation.ContentEncoding,
			ResponseSHA256: operation.ResponseSHA256, ResponseBytes: operation.ResponseBytes,
		})
	}
	if reader.manifestMutate != nil {
		reader.manifestMutate(&manifest)
	}
	return manifest, nil
}

func (reader *semanticReplayFakeEvidenceReader) ReadCognitionProviderIdentityEvidenceBody(
	_ context.Context,
	_ cognition.EpisodeID,
	id string,
	operation int,
	kind queue.CognitionProviderIdentityBodyKind,
	offset, limit int,
) (queue.CognitionProviderIdentityEvidenceBodyPage, error) {
	reader.identityRequest = append(reader.identityRequest, semanticReplayIdentityPageRequest{
		id: id, operation: operation, kind: kind, offset: offset, limit: limit,
	})
	evidence, exists := reader.identities[id]
	if !exists || operation < 0 || operation >= len(evidence.Operations) || offset < 0 || limit < 1 {
		return queue.CognitionProviderIdentityEvidenceBodyPage{}, fmt.Errorf("unexpected fake identity body")
	}
	operationEvidence := evidence.Operations[operation]
	raw, sha := operationEvidence.Request, operationEvidence.RequestSHA256
	if kind == queue.CognitionProviderIdentityResponseBody {
		raw, sha = operationEvidence.ResponseCapture, operationEvidence.ResponseSHA256
	}
	if offset > len(raw) {
		return queue.CognitionProviderIdentityEvidenceBodyPage{}, fmt.Errorf("unexpected fake identity offset")
	}
	end := offset + limit
	if end > len(raw) {
		end = len(raw)
	}
	page := queue.CognitionProviderIdentityEvidenceBodyPage{
		Ref: evidence.Ref, OperationIndex: operation, Kind: kind,
		SHA256: sha, TotalBytes: len(raw), Offset: offset, NextOffset: end,
		Content: append([]byte{}, raw[offset:end]...),
	}
	if reader.identityMutate != nil {
		reader.identityMutate(&page)
	}
	return page, nil
}

var _ ProductionSemanticReplayReader = (*semanticReplayFakeEvidenceReader)(nil)
