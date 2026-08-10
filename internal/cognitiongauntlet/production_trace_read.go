package cognitiongauntlet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
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
	return result, nil
}

func validateProductionTracePage(
	page queue.CognitionSealedTracePage,
	episode cognition.EpisodeID,
	offset int,
) error {
	if page.Schema != queue.CognitionSealedTraceSchemaV2 || page.EpisodeID != episode ||
		page.Offset != offset || page.TotalRecords < 2 || page.Records == nil ||
		!validDigest(page.TraceSHA256) || page.Seal.EpisodeID != episode ||
		page.Seal.TraceSHA256 != page.TraceSHA256 || page.Seal.FinalRevision.EpisodeID != episode ||
		page.GraphVersion == 0 || !validDigest(page.GraphSHA256) || page.LedgerVersion == 0 ||
		page.WorkingSetVersion == 0 || page.EpisodeStartedAt.IsZero() || page.SealedAt.IsZero() ||
		page.SealedAt.Before(page.EpisodeStartedAt) {
		return fmt.Errorf("production cognition trace page authority is invalid")
	}
	for index, record := range page.Records {
		if record.CallOrdinal < 0 || record.Phase < 1 || record.Phase > 100 ||
			record.Sequence < 0 || requireExact(record.Kind, "production trace kind", 128) != nil ||
			requireExact(record.ID, "production trace record ID", 512) != nil ||
			!validDigest(record.SHA256) || len(record.Payload) == 0 || !json.Valid(record.Payload) {
			return fmt.Errorf("production cognition trace record %d is invalid", offset+index)
		}
		digest := sha256.Sum256(record.Payload)
		if hex.EncodeToString(digest[:]) != record.SHA256 {
			return fmt.Errorf("production cognition trace record %d payload changed", offset+index)
		}
	}
	return nil
}

func sameProductionTraceHeader(left, right queue.CognitionSealedTracePage) bool {
	left.Offset, left.NextOffset, left.Records = 0, 0, nil
	right.Offset, right.NextOffset, right.Records = 0, 0, nil
	return reflect.DeepEqual(left, right)
}
