package cognitiongauntlet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
)

type semanticReplayFakePager struct {
	pages    map[int]queue.CognitionSealedTracePage
	requests []queue.CognitionTracePageRequest
}

func (pager *semanticReplayFakePager) ReadCognitionSealedTrace(
	_ context.Context,
	_ cognition.EpisodeID,
	request queue.CognitionTracePageRequest,
) (queue.CognitionSealedTracePage, error) {
	pager.requests = append(pager.requests, request)
	page, ok := pager.pages[request.Offset]
	if !ok {
		return queue.CognitionSealedTracePage{}, errors.New("unexpected fake trace offset")
	}
	page.Records = append([]queue.CognitionSealedTraceRecord(nil), page.Records...)
	return page, nil
}

func TestSemanticReplayPagerBindsEveryPageAndExactTraceDigest(t *testing.T) {
	pager := semanticReplayPagerFixture(t)
	trace, err := readProductionTrace(t.Context(), pager, pager.pages[0].EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSemanticProductionTraceDigest(trace); err != nil {
		t.Fatal(err)
	}
	if len(pager.requests) != 2 || pager.requests[0].Offset != 0 ||
		pager.requests[1].Offset != 1 || pager.requests[0].Limit != queue.MaxCognitionTracePageSize {
		t.Fatalf("bounded pager requests=%+v", pager.requests)
	}

	changed := semanticReplayPagerFixture(t)
	second := changed.pages[1]
	second.GraphSHA256 = strings.Repeat("9", 64)
	changed.pages[1] = second
	if _, err := readProductionTrace(t.Context(), changed, second.EpisodeID); err == nil {
		t.Fatal("trace pager accepted a changed second-page header")
	}
}

func TestSemanticReplayPagerRejectsTamperAndUnprovenClaimedDigest(t *testing.T) {
	t.Run("payload tamper", func(t *testing.T) {
		pager := semanticReplayPagerFixture(t)
		page := pager.pages[1]
		page.Records[0].Payload = json.RawMessage(`{"changed":true}`)
		pager.pages[1] = page
		if _, err := readProductionTrace(t.Context(), pager, page.EpisodeID); err == nil {
			t.Fatal("trace pager accepted payload bytes outside the sealed digest")
		}
	})
	t.Run("claimed trace digest", func(t *testing.T) {
		pager := semanticReplayPagerFixture(t)
		for offset, page := range pager.pages {
			page.TraceSHA256 = strings.Repeat("f", 64)
			page.Seal.TraceSHA256 = page.TraceSHA256
			pager.pages[offset] = page
		}
		trace, err := readProductionTrace(t.Context(), pager, pager.pages[0].EpisodeID)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateSemanticProductionTraceDigest(trace); err == nil {
			t.Fatal("adapter trusted a pager-claimed trace digest without reconstruction")
		}
	})
}

func TestSemanticReplayPagerRejectsInvalidPageCardinalityAndTerminality(t *testing.T) {
	mutations := map[string]func(*queue.CognitionSealedTracePage){
		"total above frozen maximum": func(page *queue.CognitionSealedTracePage) {
			page.TotalRecords = queue.MaxCognitionTraceRecords + 1
		},
		"page above maximum": func(page *queue.CognitionSealedTracePage) {
			page.Records = make([]queue.CognitionSealedTraceRecord, queue.MaxCognitionTracePageSize+1)
		},
		"records exceed total": func(page *queue.CognitionSealedTracePage) {
			page.TotalRecords = 1
		},
		"early terminal": func(page *queue.CognitionSealedTracePage) {
			page.NextOffset = -1
		},
		"nonterminal complete": func(page *queue.CognitionSealedTracePage) {
			page.TotalRecords = len(page.Records)
			page.NextOffset = len(page.Records)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			pager := semanticReplayPagerFixture(t)
			page := pager.pages[0]
			mutate(&page)
			pager.pages[0] = page
			if _, err := readProductionTrace(t.Context(), pager, page.EpisodeID); err == nil {
				t.Fatal("invalid sealed trace page cardinality was accepted")
			}
		})
	}
}

func TestSemanticReplayPagerRejectsPayloadAndAggregateByteOverflow(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	fixture := semanticReplayPagerFixture(t).pages[0]
	fixture.EpisodeID = episode
	t.Run("one payload", func(t *testing.T) {
		page := fixture
		payload := append([]byte{'"'}, bytes.Repeat(
			[]byte{'x'}, queue.MaxCognitionTracePayloadBytes,
		)...)
		payload = append(payload, '"')
		page.Records = []queue.CognitionSealedTraceRecord{
			semanticReplayRawRecord("transition", 0, 10, 1, "too-large", payload),
		}
		page.TotalRecords, page.NextOffset = 1, -1
		if err := validateProductionTracePage(page, episode, 0); err == nil {
			t.Fatal("oversized trace payload was accepted")
		}
	})
	t.Run("aggregate page", func(t *testing.T) {
		page := fixture
		payload := append([]byte{'"'}, bytes.Repeat(
			[]byte{'x'}, queue.MaxCognitionTracePayloadBytes-2,
		)...)
		payload = append(payload, '"')
		page.Records = make([]queue.CognitionSealedTraceRecord, 9)
		for index := range page.Records {
			page.Records[index] = semanticReplayRawRecord(
				"transition", 0, 10, int64(index+1), fmt.Sprintf("record-%d", index), payload,
			)
		}
		page.TotalRecords, page.NextOffset = len(page.Records), -1
		if err := validateProductionTracePage(page, episode, 0); err == nil {
			t.Fatal("oversized aggregate trace page was accepted")
		}
	})
}

func TestEmbeddedTraceAuthorityAcceptsMoreThanOneTransportPage(t *testing.T) {
	fixture := semanticReplayPagerFixture(t).pages[0]
	records := make([]queue.CognitionSealedTraceRecord, queue.MaxCognitionTracePageSize+1)
	for index := range records {
		records[index] = semanticReplayRawRecord(
			"transition", 0, 10, int64(index+1), fmt.Sprintf("record-%02d", index),
			[]byte(`{"schema":"fixture"}`),
		)
	}
	page, err := semanticReplayHeader(fixture).page(records)
	if err != nil || len(page.Records) != len(records) {
		t.Fatalf("embedded multi-page trace records=%d err=%v", len(page.Records), err)
	}
}

func semanticReplayPagerFixture(t *testing.T) *semanticReplayFakePager {
	t.Helper()
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	revision, err := cognition.NewWorldRevision(episode, 1, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	records := []queue.CognitionSealedTraceRecord{
		semanticReplayRawRecord("transition", 0, 10, 1, "transition-1", []byte(`{}`)),
		semanticReplayRawRecord("obligation_graph", 0, 20, 1, "graph-1", []byte(`{}`)),
	}
	started := time.Date(2026, time.August, 12, 1, 0, 0, 0, time.UTC)
	header := queue.CognitionSealedTracePage{
		Schema: queue.CognitionSealedTraceSchemaV2, EpisodeID: episode,
		GraphVersion: 1, GraphSHA256: strings.Repeat("c", 64), LedgerVersion: 1,
		WorkingSetVersion: 1, EpisodeStartedAt: started, SealedAt: started.Add(time.Second),
		TotalRecords: len(records),
	}
	header.Seal = queue.CognitionTerminalSeal{
		EpisodeID: episode, Outcome: queue.CognitionEpisodeCompleted, FinalRevision: revision,
		ObligationGraphSHA256: header.GraphSHA256, LedgerVersion: 1, WorkingSetVersion: 1,
		CreatedAt: header.SealedAt,
	}
	header.TraceSHA256 = semanticReplayTraceDigestForTest(t, header, records)
	header.Seal.TraceSHA256 = header.TraceSHA256
	first, second := header, header
	first.Offset, first.NextOffset, first.Records = 0, 1, records[:1]
	second.Offset, second.NextOffset, second.Records = 1, -1, records[1:]
	return &semanticReplayFakePager{pages: map[int]queue.CognitionSealedTracePage{0: first, 1: second}}
}

func semanticReplayTraceDigestForTest(
	t *testing.T,
	header queue.CognitionSealedTracePage,
	records []queue.CognitionSealedTraceRecord,
) string {
	t.Helper()
	authorityRecords := make([]semanticProductionTraceRecord, len(records))
	for index, record := range records {
		authorityRecords[index] = semanticProductionTraceRecord{
			Kind: record.Kind, CallOrdinal: record.CallOrdinal, Phase: record.Phase,
			Sequence: record.Sequence, ID: record.ID, SHA256: record.SHA256,
		}
	}
	raw, err := json.Marshal(semanticProductionTraceAuthority{
		Schema: semanticProductionTraceAuthoritySchemaV2, EpisodeID: header.EpisodeID,
		Revision: header.Seal.FinalRevision, GraphVersion: header.GraphVersion,
		GraphSHA256: header.GraphSHA256, LedgerVersion: header.LedgerVersion,
		WorkingVersion: header.WorkingSetVersion, Records: authorityRecords,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func semanticReplayRawRecord(
	kind string,
	call int64,
	phase int,
	sequence int64,
	id string,
	payload []byte,
) queue.CognitionSealedTraceRecord {
	digest := sha256.Sum256(payload)
	return queue.CognitionSealedTraceRecord{
		Kind: kind, CallOrdinal: call, Phase: phase, Sequence: sequence, ID: id,
		SHA256: hex.EncodeToString(digest[:]), Payload: append(json.RawMessage(nil), payload...),
	}
}
