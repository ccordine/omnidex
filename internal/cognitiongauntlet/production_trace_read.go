package cognitiongauntlet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

type sealedTraceReader interface {
	ReadCognitionSealedTrace(
		context.Context,
		cognition.EpisodeID,
		queue.CognitionTracePageRequest,
	) (queue.CognitionSealedTracePage, error)
}

type productionTrace struct {
	Header  queue.CognitionSealedTracePage
	Records []queue.CognitionSealedTraceRecord
}

func readProductionTrace(
	ctx context.Context,
	reader sealedTraceReader,
	episode cognition.EpisodeID,
) (productionTrace, error) {
	if ctx == nil || reader == nil {
		return productionTrace{}, fmt.Errorf("production trace requires context and a sealed queue reader")
	}
	result := productionTrace{}
	offset := 0
	remainingPayloadBytes := cognitionreplay.MaxContainerBytes
	for {
		page, err := reader.ReadCognitionSealedTrace(ctx, episode, queue.CognitionTracePageRequest{
			Offset: offset, Limit: queue.MaxCognitionTracePageSize,
		})
		if err != nil {
			return productionTrace{}, fmt.Errorf("read production cognition trace at offset %d: %w", offset, err)
		}
		if err := validateProductionTracePage(page, episode, offset); err != nil {
			return productionTrace{}, err
		}
		if offset == 0 {
			result.Header = page
		} else if !sameProductionTraceHeader(result.Header, page) {
			return productionTrace{}, fmt.Errorf("production cognition trace header changed between pages")
		}
		if err := consumeProductionTracePayloadBudget(
			page.Records, &remainingPayloadBytes,
		); err != nil {
			return productionTrace{}, err
		}
		result.Records = append(result.Records, page.Records...)
		if page.NextOffset == -1 {
			break
		}
		if page.NextOffset <= offset || page.NextOffset != offset+len(page.Records) {
			return productionTrace{}, fmt.Errorf("production cognition trace page did not advance exactly")
		}
		offset = page.NextOffset
	}
	if len(result.Records) != result.Header.TotalRecords {
		return productionTrace{}, fmt.Errorf("production cognition trace record count is incomplete")
	}
	if err := queue.VerifyCognitionSealedTraceRecordOrder(result.Records); err != nil {
		return productionTrace{}, fmt.Errorf("production cognition trace order: %w", err)
	}
	return result, nil
}

func consumeProductionTracePayloadBudget(
	records []queue.CognitionSealedTraceRecord,
	remaining *int,
) error {
	if remaining == nil || *remaining < 0 {
		return fmt.Errorf("production cognition trace payload budget is invalid")
	}
	for _, record := range records {
		if len(record.Payload) > *remaining {
			return fmt.Errorf("production cognition trace exceeds the replay container bound")
		}
		*remaining -= len(record.Payload)
	}
	return nil
}

func validateProductionTracePage(
	page queue.CognitionSealedTracePage,
	episode cognition.EpisodeID,
	offset int,
) error {
	if validateProductionTraceHeader(page, episode) != nil ||
		page.Offset != offset || page.TotalRecords < 2 ||
		page.TotalRecords > queue.MaxCognitionTraceRecords || page.Records == nil ||
		len(page.Records) == 0 || len(page.Records) > queue.MaxCognitionTracePageSize ||
		offset+len(page.Records) > page.TotalRecords ||
		(page.NextOffset == -1) != (offset+len(page.Records) == page.TotalRecords) {
		return fmt.Errorf("production cognition trace page authority is invalid")
	}
	remaining := queue.MaxCognitionTracePageBytes
	for index, record := range page.Records {
		if validateProductionTraceRecord(record, offset+index) != nil ||
			len(record.Payload) > remaining {
			return fmt.Errorf("production cognition trace record %d is invalid", offset+index)
		}
		remaining -= len(record.Payload)
	}
	return nil
}

func validateProductionTraceHeader(
	page queue.CognitionSealedTracePage,
	episode cognition.EpisodeID,
) error {
	if page.Schema != queue.CognitionSealedTraceSchemaV2 || page.EpisodeID != episode ||
		!validDigest(page.TraceSHA256) || page.Seal.EpisodeID != episode ||
		page.Seal.TraceSHA256 != page.TraceSHA256 || page.Seal.FinalRevision.EpisodeID != episode ||
		page.GraphVersion == 0 || !validDigest(page.GraphSHA256) || page.LedgerVersion == 0 ||
		page.WorkingSetVersion == 0 || validateProductionSemanticReplayTimes(
		page.EpisodeStartedAt, page.SealedAt, page.Seal.CreatedAt,
	) != nil {
		return fmt.Errorf("production cognition trace header authority is invalid")
	}
	return nil
}

func validateProductionTraceRecord(
	record queue.CognitionSealedTraceRecord,
	index int,
) error {
	if record.CallOrdinal < 0 || record.Phase < 1 || record.Phase > 100 ||
		record.Sequence < 0 || requireExact(record.Kind, "production trace kind", 128) != nil ||
		requireExact(record.ID, "production trace record ID", 512) != nil ||
		!validDigest(record.SHA256) || len(record.Payload) == 0 ||
		len(record.Payload) > queue.MaxCognitionTracePayloadBytes || !json.Valid(record.Payload) {
		return fmt.Errorf("production cognition trace record %d is invalid", index)
	}
	digest := sha256.Sum256(record.Payload)
	if hex.EncodeToString(digest[:]) != record.SHA256 {
		return fmt.Errorf("production cognition trace record %d payload changed", index)
	}
	return nil
}

func sameProductionTraceHeader(left, right queue.CognitionSealedTracePage) bool {
	left.Offset, left.NextOffset, left.Records = 0, 0, nil
	right.Offset, right.NextOffset, right.Records = 0, 0, nil
	return reflect.DeepEqual(left, right)
}
