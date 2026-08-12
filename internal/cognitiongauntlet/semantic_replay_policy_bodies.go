package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

type semanticPolicyPageReader func(
	context.Context, cognition.EpisodeID, string, int, int,
) (queue.CognitionPolicyEvidencePage, error)

func readSemanticPolicyBodies(
	ctx context.Context,
	reader ProductionSemanticReplayReader,
	episodeID cognition.EpisodeID,
	trace productionTrace,
	inventory semanticReplayEvidenceInventory,
	supplement *semanticReplaySupplement,
) error {
	if err := preflightSemanticPolicyBodies(trace, inventory); err != nil {
		return err
	}
	for _, record := range trace.Records {
		kind := semanticPolicyEvidenceKind(record.Kind)
		if kind == "" {
			continue
		}
		metadata := inventory.policy[semanticReplayEvidenceKey(kind, record.ID)]
		raw, err := readSemanticPolicyBody(ctx, reader, episodeID, metadata)
		if err != nil {
			return err
		}
		content, chunked, blobs, err := semanticReplayPolicyBodyContent(kind, metadata, raw)
		if err != nil {
			return err
		}
		if err := supplement.add(
			semanticPolicySidecarKind(kind), metadata.EvidenceID,
			content, chunked, blobs,
		); err != nil {
			return err
		}
		if err := supplement.addPolicyBody(kind, metadata.EvidenceID, raw); err != nil {
			return err
		}
	}
	return nil
}

func preflightSemanticPolicyBodies(
	trace productionTrace,
	inventory semanticReplayEvidenceInventory,
) error {
	remaining := cognitionreplay.MaxContainerBytes
	for _, record := range trace.Records {
		kind := semanticPolicyEvidenceKind(record.Kind)
		if kind == "" {
			continue
		}
		metadata, exists := inventory.policy[semanticReplayEvidenceKey(kind, record.ID)]
		if !exists || validateSemanticPolicyEvidenceBytes(metadata) != nil ||
			metadata.Bytes > remaining {
			return fmt.Errorf("semantic policy evidence bodies exceed their replay bound")
		}
		remaining -= metadata.Bytes
	}
	return nil
}

func readSemanticPolicyBody(
	ctx context.Context,
	reader ProductionSemanticReplayReader,
	episodeID cognition.EpisodeID,
	metadata semanticPolicyEvidence,
) ([]byte, error) {
	if err := validateSemanticPolicyEvidenceBytes(metadata); err != nil {
		return nil, err
	}
	read := semanticPolicyEvidenceReader(reader, metadata.EvidenceKind)
	if read == nil {
		return nil, fmt.Errorf("semantic policy evidence kind is not registered")
	}
	capacity := metadata.Bytes
	if capacity > queue.MaxCognitionPolicyEvidencePageBytes {
		capacity = queue.MaxCognitionPolicyEvidencePageBytes
	}
	raw := make([]byte, 0, capacity)
	offset := 0
	for {
		page, err := read(
			ctx, episodeID, metadata.EvidenceID, offset,
			queue.MaxCognitionPolicyEvidencePageBytes,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"read semantic %s evidence %q at %d: %w",
				metadata.EvidenceKind, metadata.EvidenceID, offset, err,
			)
		}
		if err := validateSemanticPolicyPage(page, metadata, offset); err != nil {
			return nil, err
		}
		raw = append(raw, page.Content...)
		if page.NextOffset == metadata.Bytes {
			break
		}
		offset = page.NextOffset
	}
	if len(raw) != metadata.Bytes || digestExactBytes(raw) != metadata.ContentSHA256 {
		return nil, fmt.Errorf("semantic policy evidence body changed across its pages")
	}
	return raw, nil
}

func semanticPolicyEvidenceReader(
	reader ProductionSemanticReplayReader,
	kind string,
) semanticPolicyPageReader {
	switch kind {
	case "model_response":
		return reader.ReadCognitionModelResponseEvidence
	case "provider_generation":
		return reader.ReadCognitionProviderGenerationEvidence
	case "provider_response_capture":
		return reader.ReadCognitionProviderResponseCapture
	default:
		return nil
	}
}

func validateSemanticPolicyPage(
	page queue.CognitionPolicyEvidencePage,
	metadata semanticPolicyEvidence,
	offset int,
) error {
	if page.CallID != metadata.CallID || page.EvidenceID != metadata.EvidenceID ||
		page.SHA256 != metadata.ContentSHA256 || page.TotalBytes != metadata.Bytes ||
		page.Offset != offset || page.NextOffset != offset+len(page.Content) ||
		len(page.Content) > queue.MaxCognitionPolicyEvidencePageBytes ||
		page.NextOffset > metadata.Bytes {
		return fmt.Errorf("semantic policy evidence page authority changed")
	}
	if metadata.Bytes == 0 {
		if metadata.EvidenceKind != "provider_response_capture" || offset != 0 ||
			len(page.Content) != 0 || page.NextOffset != 0 {
			return fmt.Errorf("semantic empty provider response capture page changed")
		}
		return nil
	}
	if len(page.Content) == 0 || page.NextOffset <= offset {
		return fmt.Errorf("semantic policy evidence pager made no progress")
	}
	return nil
}

func semanticReplayContentForBytes(
	id string,
	raw []byte,
) (
	cognitionreplay.ProjectionContentAuthority,
	[]cognitionreplay.ChunkedBlobBinding,
	[]cognitionreplay.Blob,
	error,
) {
	if len(raw) == 0 {
		content, err := cognitionreplay.NewEmptyProjectionContent("application/octet-stream")
		return content, []cognitionreplay.ChunkedBlobBinding{}, []cognitionreplay.Blob{}, err
	}
	return cognitionreplay.NewPublicProjectionContent(id, "application/octet-stream", raw)
}
