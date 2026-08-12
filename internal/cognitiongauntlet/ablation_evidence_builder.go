package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/llm"
)

type ablationContentBuilder struct {
	private  bool
	bindings map[string]cognitionreplay.ChunkedBlobBinding
	blobs    map[string]cognitionreplay.Blob
}

func newAblationContentBuilder(private bool) *ablationContentBuilder {
	return &ablationContentBuilder{
		private: private, bindings: make(map[string]cognitionreplay.ChunkedBlobBinding),
		blobs: make(map[string]cognitionreplay.Blob),
	}
}

func (builder *ablationContentBuilder) content(
	id, mediaType string,
	raw []byte,
	allowEmpty bool,
) (cognitionreplay.ProjectionContentAuthority, error) {
	var authority cognitionreplay.ProjectionContentAuthority
	var bindings []cognitionreplay.ChunkedBlobBinding
	var blobs []cognitionreplay.Blob
	var err error
	if len(raw) == 0 {
		if !allowEmpty {
			return cognitionreplay.ProjectionContentAuthority{}, fmt.Errorf(
				"ablation evidence %q cannot be empty", id,
			)
		}
		if builder.private {
			authority, err = cognitionreplay.NewPrivateEmptyProjectionContent(mediaType)
		} else {
			authority, err = cognitionreplay.NewEmptyProjectionContent(mediaType)
		}
	} else if builder.private {
		authority, bindings, blobs, err = cognitionreplay.NewPrivateProjectionContent(id, mediaType, raw)
	} else {
		authority, bindings, blobs, err = cognitionreplay.NewPublicProjectionContent(id, mediaType, raw)
	}
	if err != nil {
		return cognitionreplay.ProjectionContentAuthority{}, err
	}
	for _, binding := range bindings {
		builder.bindings[binding.Manifest.SHA256] = binding
	}
	for _, blob := range blobs {
		if existing, duplicate := builder.blobs[blob.SHA256]; duplicate &&
			(existing.MediaType != blob.MediaType || !bytes.Equal(existing.Data, blob.Data)) {
			return cognitionreplay.ProjectionContentAuthority{}, fmt.Errorf(
				"ablation evidence reused one content digest with different bytes",
			)
		}
		builder.blobs[blob.SHA256] = blob
	}
	return authority, nil
}

func buildAblationCalls(
	calls []ablationCall,
	builder *ablationContentBuilder,
) ([]ablationCallEvidence, error) {
	values := make([]ablationCallEvidence, len(calls))
	for index, call := range calls {
		projectionContent, err := builder.content(
			fmt.Sprintf("ablation-projection-%03d", index+1),
			"text/plain; charset=utf-8", []byte(call.Projection.Rendered), false,
		)
		if err != nil {
			return nil, err
		}
		evidence, err := buildAblationPolicyEvidence(call, builder)
		if err != nil {
			return nil, fmt.Errorf("ablation call %d: %w", index+1, err)
		}
		values[index] = ablationCallEvidence{
			Ordinal: uint32(index + 1),
			Projection: ablationProjectionEvidence{
				Projection: cloneAblationProjection(call.Projection), Content: projectionContent,
			},
			Snapshot: call.Snapshot.clone(), Attempt: call.Attempt.Clone(),
			Result: call.Result.Clone(), Evidence: evidence,
		}
	}
	return values, nil
}

func buildAblationPolicyEvidence(
	call ablationCall,
	builder *ablationContentBuilder,
) (ablationPolicyEvidence, error) {
	value := ablationPolicyEvidence{}
	if call.Result.ResponseEvidence != (cognitionpolicy.ModelResponseEvidenceRef{}) {
		content, err := builder.content(
			"model-response-"+call.Attempt.ID, "application/octet-stream",
			call.Evidence.Response.Content, false,
		)
		if err != nil {
			return value, err
		}
		value.ModelResponse = &ablationModelResponseEvidence{
			Ref: call.Evidence.Response.Ref, CallID: call.Evidence.Response.CallID, Content: content,
		}
	}
	if call.Result.ProviderIdentityEvidence != (llm.ProviderIdentityEvidenceRef{}) {
		identity, err := buildAblationIdentityEvidence(call.Evidence.ProviderIdentity, builder)
		if err != nil {
			return value, err
		}
		value.ProviderIdentity = &identity
	}
	if call.Result.ProviderResponseCapture != (cognitionpolicy.ProviderResponseCaptureEvidenceRef{}) {
		content, err := builder.content(
			"provider-capture-"+call.Attempt.ID, "application/octet-stream",
			call.Evidence.ProviderResponseCapture.Content, true,
		)
		if err != nil {
			return value, err
		}
		value.ProviderResponseCapture = &ablationProviderCaptureEvidence{
			Ref:    call.Evidence.ProviderResponseCapture.Ref,
			CallID: call.Evidence.ProviderResponseCapture.CallID, Content: content,
		}
	}
	if call.Result.ProviderGenerationEvidence != (cognitionpolicy.ProviderGenerationEvidenceRef{}) {
		content, err := builder.content(
			"provider-generation-"+call.Attempt.ID, "application/octet-stream",
			call.Evidence.ProviderGeneration.Generation, false,
		)
		if err != nil {
			return value, err
		}
		value.ProviderGeneration = &ablationProviderGenerationEvidence{
			Ref:    call.Evidence.ProviderGeneration.Ref,
			CallID: call.Evidence.ProviderGeneration.CallID, Content: content,
		}
	}
	return value, nil
}

func buildAblationIdentityEvidence(
	evidence llm.ProviderIdentityEvidence,
	builder *ablationContentBuilder,
) (ablationIdentityEvidence, error) {
	value := ablationIdentityEvidence{
		Schema: evidence.Schema, Ref: evidence.Ref,
		Operations: make([]ablationIdentityOperationEvidence, len(evidence.Operations)),
	}
	for index, operation := range evidence.Operations {
		request, err := builder.content(
			fmt.Sprintf("provider-identity-%s-request", operation.Operation),
			"application/octet-stream", operation.Request, true,
		)
		if err != nil {
			return ablationIdentityEvidence{}, err
		}
		response, err := builder.content(
			fmt.Sprintf("provider-identity-%s-response", operation.Operation),
			"application/octet-stream", operation.ResponseCapture, true,
		)
		if err != nil {
			return ablationIdentityEvidence{}, err
		}
		value.Operations[index] = ablationIdentityOperationEvidence{
			Operation: operation.Operation, Method: operation.Method, Endpoint: operation.Endpoint,
			RequestDisposition: operation.RequestDisposition,
			RequestSHA256:      operation.RequestSHA256, RequestBytes: operation.RequestBytes,
			RequestContent: request, HTTPStatus: operation.HTTPStatus,
			Disposition: operation.Disposition, ResponseComplete: operation.ResponseComplete,
			ContentEncoding: operation.ContentEncoding,
			ResponseSHA256:  operation.ResponseSHA256, ResponseBytes: operation.ResponseBytes,
			ResponseContent: response,
		}
	}
	return value, nil
}

func encodeAblationEvidenceArtifact(artifact ablationEvidenceArtifact) ([]byte, error) {
	raw, err := json.Marshal(artifact)
	if err != nil {
		return nil, fmt.Errorf("encode ablation evidence: %w", err)
	}
	return append(raw, '\n'), nil
}
