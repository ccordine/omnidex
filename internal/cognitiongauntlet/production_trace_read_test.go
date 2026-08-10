package cognitiongauntlet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type pagedTraceReader struct {
	pages   map[int]queue.CognitionSealedTracePage
	offsets []int
}

func (reader *pagedTraceReader) ReadCognitionSealedTrace(
	_ context.Context,
	_ cognition.EpisodeID,
	request queue.CognitionTracePageRequest,
) (queue.CognitionSealedTracePage, error) {
	reader.offsets = append(reader.offsets, request.Offset)
	page, exists := reader.pages[request.Offset]
	if !exists {
		return queue.CognitionSealedTracePage{}, errors.New("unexpected offset")
	}
	return page, nil
}

func TestReadProductionTraceRequiresExactVerifiedPagination(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + traceTestDigest("episode"))
	first := productionTraceTestPage(t, episode, 0, 1)
	second := productionTraceTestPage(t, episode, 1, -1)
	first.Records = []queue.CognitionSealedTraceRecord{productionTraceTestRecord("transition", "one")}
	second.Records = []queue.CognitionSealedTraceRecord{productionTraceTestRecord("obligation_graph", "two")}
	reader := &pagedTraceReader{pages: map[int]queue.CognitionSealedTracePage{0: first, 1: second}}

	trace, err := readProductionTrace(context.Background(), reader, episode)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Records) != 2 || len(reader.offsets) != 2 || reader.offsets[1] != 1 {
		t.Fatalf("trace=%#v offsets=%v", trace, reader.offsets)
	}
}

func TestReadProductionTraceRejectsChangedPayloadOrHeader(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + traceTestDigest("episode"))
	page := productionTraceTestPage(t, episode, 0, -1)
	page.Records = []queue.CognitionSealedTraceRecord{
		productionTraceTestRecord("transition", "one"),
		productionTraceTestRecord("obligation_graph", "two"),
	}
	page.Records[1].Payload = []byte(`{"changed":true}`)
	reader := &pagedTraceReader{pages: map[int]queue.CognitionSealedTracePage{0: page}}
	if _, err := readProductionTrace(context.Background(), reader, episode); err == nil {
		t.Fatal("payload changed after queue sealing was accepted")
	}

	first := productionTraceTestPage(t, episode, 0, 1)
	second := productionTraceTestPage(t, episode, 1, -1)
	first.Records = []queue.CognitionSealedTraceRecord{productionTraceTestRecord("transition", "one")}
	second.Records = []queue.CognitionSealedTraceRecord{productionTraceTestRecord("obligation_graph", "two")}
	second.GraphVersion++
	reader = &pagedTraceReader{pages: map[int]queue.CognitionSealedTracePage{0: first, 1: second}}
	if _, err := readProductionTrace(context.Background(), reader, episode); err == nil {
		t.Fatal("header changed between queue trace pages was accepted")
	}

	missingTime := productionTraceTestPage(t, episode, 0, -1)
	missingTime.Records = []queue.CognitionSealedTraceRecord{
		productionTraceTestRecord("transition", "one"),
		productionTraceTestRecord("obligation_graph", "two"),
	}
	missingTime.EpisodeStartedAt = time.Time{}
	reader = &pagedTraceReader{pages: map[int]queue.CognitionSealedTracePage{0: missingTime}}
	if _, err := readProductionTrace(context.Background(), reader, episode); err == nil {
		t.Fatal("trace with missing durable episode start time was accepted")
	}
}

func productionTraceTestPage(
	t *testing.T,
	episode cognition.EpisodeID,
	offset, next int,
) queue.CognitionSealedTracePage {
	t.Helper()
	revision, err := cognition.NewWorldRevision(episode, 2, traceTestDigest("revision"))
	if err != nil {
		t.Fatal(err)
	}
	traceSHA := traceTestDigest("trace")
	startedAt := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	return queue.CognitionSealedTracePage{
		Schema: queue.CognitionSealedTraceSchemaV2, EpisodeID: episode,
		TraceSHA256: traceSHA,
		Seal: queue.CognitionTerminalSeal{
			EpisodeID: episode, Outcome: queue.CognitionEpisodeCompleted,
			FinalRevision: revision, CompletionSHA256: traceTestDigest("completion"),
			ObligationGraphSHA256: traceTestDigest("graph"), LedgerVersion: 2,
			WorkingSetVersion: 2, TraceSHA256: traceSHA,
			SealedBy: model.StepAttemptAuthority{
				JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker",
			},
		},
		GraphVersion: 2, GraphSHA256: traceTestDigest("graph"), LedgerVersion: 2,
		WorkingSetVersion: 2, EpisodeStartedAt: startedAt,
		SealedAt:     startedAt.Add(250 * time.Millisecond),
		TotalRecords: 2, Offset: offset, NextOffset: next,
	}
}

func productionTraceTestRecord(kind, id string) queue.CognitionSealedTraceRecord {
	payload := []byte(`{"value":"` + id + `"}`)
	digest := sha256.Sum256(payload)
	return queue.CognitionSealedTraceRecord{
		Kind: kind, Phase: 10, ID: id, SHA256: hex.EncodeToString(digest[:]), Payload: payload,
	}
}

func traceTestDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
