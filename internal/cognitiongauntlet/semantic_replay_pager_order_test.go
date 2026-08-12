package cognitiongauntlet

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticReplayPagerRejectsCanonicalOrderMutation(t *testing.T) {
	t.Run("same page", func(t *testing.T) {
		pager := semanticReplayPagerFixture(t)
		first, second := pager.pages[0], pager.pages[1]
		reordered := []queue.CognitionSealedTraceRecord{
			second.Records[0], first.Records[0],
		}
		first.Records = reordered
		first.TotalRecords, first.NextOffset = len(reordered), -1
		first.TraceSHA256 = semanticReplayTraceDigestForTest(t, first, reordered)
		first.Seal.TraceSHA256 = first.TraceSHA256
		pager.pages = map[int]queue.CognitionSealedTracePage{0: first}

		if _, err := readProductionTrace(t.Context(), pager, first.EpisodeID); err == nil {
			t.Fatal("self-consistent same-page record reorder was accepted")
		}
	})

	t.Run("page boundary", func(t *testing.T) {
		pager := semanticReplayPagerFixture(t)
		first, second := pager.pages[0], pager.pages[1]
		first.Records[0], second.Records[0] = second.Records[0], first.Records[0]
		reordered := []queue.CognitionSealedTraceRecord{
			first.Records[0], second.Records[0],
		}
		traceSHA := semanticReplayTraceDigestForTest(t, first, reordered)
		first.TraceSHA256, first.Seal.TraceSHA256 = traceSHA, traceSHA
		second.TraceSHA256, second.Seal.TraceSHA256 = traceSHA, traceSHA
		pager.pages[0], pager.pages[1] = first, second

		if _, err := readProductionTrace(t.Context(), pager, first.EpisodeID); err == nil {
			t.Fatal("self-consistent cross-page record reorder was accepted")
		}
	})
}

func TestEmbeddedSemanticTraceRejectsCanonicalOrderMutation(t *testing.T) {
	pager := semanticReplayPagerFixture(t)
	first, second := pager.pages[0], pager.pages[1]
	reordered := []queue.CognitionSealedTraceRecord{
		second.Records[0], first.Records[0],
	}
	first.TraceSHA256 = semanticReplayTraceDigestForTest(t, first, reordered)
	first.Seal.TraceSHA256 = first.TraceSHA256
	trace := productionTrace{Header: first, Records: reordered}
	if err := validateSemanticProductionTraceDigest(trace); err == nil {
		t.Fatal("rehashed embedded production trace accepted a noncanonical record order")
	}
}

func TestProductionTracePayloadBudgetRejectsBeforeAccumulation(t *testing.T) {
	remaining := 2
	records := []queue.CognitionSealedTraceRecord{{Payload: []byte(`{}`)}}
	if err := consumeProductionTracePayloadBudget(records, &remaining); err != nil || remaining != 0 {
		t.Fatalf("exact trace byte budget remaining=%d err=%v", remaining, err)
	}
	if err := consumeProductionTracePayloadBudget(records, &remaining); err == nil {
		t.Fatal("trace reader accepted payload bytes beyond its container bound")
	}
	remaining = cognitionreplay.MaxContainerBytes
	if err := consumeProductionTracePayloadBudget(nil, &remaining); err != nil ||
		remaining != cognitionreplay.MaxContainerBytes {
		t.Fatalf("empty trace page changed byte budget: remaining=%d err=%v", remaining, err)
	}
}
