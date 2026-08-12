package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func prepareProviderFailureReplayEvidence(
	ctx context.Context,
	repository *queue.Repository,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	record queue.CognitionProviderActivationFailureRecord,
) (
	cognitionreplay.ProviderIdentityEvidenceReplay,
	[]cognitionreplay.SourceRecord,
	[]cognitionreplay.ChunkedBlobBinding,
	[]cognitionreplay.Blob,
	error,
) {
	if record.Evidence.Ref.ID == "" || len(record.Evidence.Operations) != 5 {
		return cognitionreplay.ProviderIdentityEvidenceReplay{}, nil, nil, nil,
			fmt.Errorf("provider failure replay evidence manifest is incomplete")
	}
	replay := cognitionreplay.ProviderIdentityEvidenceReplay{
		Schema:     cognitionreplay.ProviderIdentityEvidenceReplaySchemaV1,
		Ref:        record.Evidence.Ref,
		Operations: make([]cognitionreplay.ProviderIdentityReplayOperation, 5),
	}
	bodySources := make([]cognitionreplay.SourceRecord, 0, 10)
	chunked := make([]cognitionreplay.ChunkedBlobBinding, 0, 10)
	blobs := make([]cognitionreplay.Blob, 0, 30)
	nextOrdinal := uint64(5)
	for index, metadata := range record.Evidence.Operations {
		if metadata.Index != index {
			return cognitionreplay.ProviderIdentityEvidenceReplay{}, nil, nil, nil,
				fmt.Errorf("provider failure replay evidence operation order changed")
		}
		request, err := readProviderFailureReplayBody(
			ctx, repository, authority, episodeID, record.RecordID,
			record.Evidence.Ref.ID, index, queue.CognitionProviderIdentityRequestBody,
			metadata.RequestSHA256, metadata.RequestBytes,
		)
		if err != nil {
			return cognitionreplay.ProviderIdentityEvidenceReplay{}, nil, nil, nil, err
		}
		requestBinding, source, binding, values, err := prepareProviderFailureBody(
			record.Evidence.Ref.ID, index, "request", request, metadata.RequestSHA256,
			metadata.RequestBytes, nextOrdinal,
		)
		if err != nil {
			return cognitionreplay.ProviderIdentityEvidenceReplay{}, nil, nil, nil, err
		}
		if source != nil {
			bodySources = append(bodySources, *source)
			nextOrdinal++
		}
		if binding != nil {
			chunked = append(chunked, *binding)
		}
		blobs = append(blobs, values...)
		response, err := readProviderFailureReplayBody(
			ctx, repository, authority, episodeID, record.RecordID,
			record.Evidence.Ref.ID, index, queue.CognitionProviderIdentityResponseBody,
			metadata.ResponseSHA256, metadata.ResponseBytes,
		)
		if err != nil {
			return cognitionreplay.ProviderIdentityEvidenceReplay{}, nil, nil, nil, err
		}
		responseBinding, source, binding, values, err := prepareProviderFailureBody(
			record.Evidence.Ref.ID, index, "response", response, metadata.ResponseSHA256,
			metadata.ResponseBytes, nextOrdinal,
		)
		if err != nil {
			return cognitionreplay.ProviderIdentityEvidenceReplay{}, nil, nil, nil, err
		}
		if source != nil {
			bodySources = append(bodySources, *source)
			nextOrdinal++
		}
		if binding != nil {
			chunked = append(chunked, *binding)
		}
		blobs = append(blobs, values...)
		replay.Operations[index] = cognitionreplay.ProviderIdentityReplayOperation{
			Index: index, Operation: metadata.Operation, Method: metadata.Method,
			Endpoint: metadata.Endpoint, RequestDisposition: metadata.RequestDisposition,
			Request: requestBinding, HTTPStatus: metadata.HTTPStatus,
			Disposition: metadata.Disposition, ResponseComplete: metadata.ResponseComplete,
			ContentEncoding: metadata.ContentEncoding, Response: responseBinding,
		}
	}
	if err := replay.Validate(); err != nil {
		return cognitionreplay.ProviderIdentityEvidenceReplay{}, nil, nil, nil, err
	}
	return replay, bodySources, chunked, blobs, nil
}

func readProviderFailureReplayBody(
	ctx context.Context,
	repository *queue.Repository,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	recordID string,
	evidenceID string,
	operation int,
	kind queue.CognitionProviderIdentityBodyKind,
	wantSHA string,
	wantBytes int,
) ([]byte, error) {
	result := make([]byte, 0, wantBytes)
	for offset := 0; ; {
		page, err := repository.ReadCognitionProviderActivationFailureBody(
			ctx, queue.CognitionProviderActivationFailureBodyRequest{
				Authority: authority, EpisodeID: episodeID, RecordID: recordID,
				EvidenceID: evidenceID, OperationIndex: operation, Kind: kind,
				Offset: offset, Limit: queue.MaxCognitionPolicyEvidencePageBytes,
			},
		)
		if err != nil {
			return nil, err
		}
		if page.Ref.ID != evidenceID || page.OperationIndex != operation || page.Kind != kind ||
			page.SHA256 != wantSHA || page.TotalBytes != wantBytes || page.Offset != offset ||
			page.NextOffset != offset+len(page.Content) || page.NextOffset > wantBytes {
			return nil, fmt.Errorf("provider failure replay body page authority changed")
		}
		result = append(result, page.Content...)
		if page.NextOffset == wantBytes {
			if len(result) != wantBytes || digestExactBytes(result) != wantSHA {
				return nil, fmt.Errorf("provider failure replay body changed")
			}
			return result, nil
		}
		if page.NextOffset <= offset {
			return nil, fmt.Errorf("provider failure replay body page did not advance")
		}
		offset = page.NextOffset
	}
}

func prepareProviderFailureBody(
	evidenceID string,
	operation int,
	kind string,
	raw []byte,
	wantSHA string,
	wantBytes int,
	ordinal uint64,
) (
	cognitionreplay.ProviderIdentityBodyBinding,
	*cognitionreplay.SourceRecord,
	*cognitionreplay.ChunkedBlobBinding,
	[]cognitionreplay.Blob,
	error,
) {
	if len(raw) != wantBytes || digestExactBytes(raw) != wantSHA {
		return cognitionreplay.ProviderIdentityBodyBinding{}, nil, nil, nil,
			fmt.Errorf("provider failure replay body metadata changed")
	}
	value := cognitionreplay.ProviderIdentityBodyBinding{
		SHA256: wantSHA, ByteCount: wantBytes,
	}
	if wantBytes == 0 {
		value.Storage = cognitionreplay.EvidenceBodyEmpty
		return value, nil, nil, nil, nil
	}
	id := fmt.Sprintf("%s:%d:%s", evidenceID, operation, kind)
	wantKind := cognitionreplay.SourceProviderIdentityRequestBody
	phase := 1
	if kind == "response" {
		wantKind = cognitionreplay.SourceProviderIdentityResponseBody
		phase = 2
	} else if kind != "request" {
		return cognitionreplay.ProviderIdentityBodyBinding{}, nil, nil, nil,
			fmt.Errorf("provider failure replay body kind is invalid")
	}
	var payload cognitionreplay.BlobRef
	var binding *cognitionreplay.ChunkedBlobBinding
	var blobs []cognitionreplay.Blob
	if wantBytes <= cognitionreplay.MaxDirectBlobBytes {
		blob, err := cognitionreplay.NewBlob("application/octet-stream", raw)
		if err != nil {
			return cognitionreplay.ProviderIdentityBodyBinding{}, nil, nil, nil, err
		}
		payload, blobs = blob.Ref(), []cognitionreplay.Blob{blob}
	} else {
		chunked, values, err := cognitionreplay.ChunkPublicBytes(id, "application/octet-stream", raw)
		if err != nil {
			return cognitionreplay.ProviderIdentityBodyBinding{}, nil, nil, nil, err
		}
		payload, binding, blobs = chunked.Manifest, &chunked, values
	}
	source := cognitionreplay.SourceRecord{
		Ordinal: ordinal, CallOrdinal: int64(operation + 1), Phase: phase, Sequence: 1,
		Kind: wantKind, ID: id, Payload: payload,
	}
	ref := source.Ref()
	value.Storage, value.Source = cognitionreplay.EvidenceBodySource, &ref
	return value, &source, binding, blobs, nil
}
