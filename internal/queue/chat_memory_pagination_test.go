package queue

import (
	"context"
	"testing"
)

func TestChatMemoryPaginationRejectsInvalidBoundsBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	repository := &Repository{}
	for _, bounds := range [][2]int{{0, 0}, {101, 0}, {10, -1}} {
		if _, err := repository.ListMemoryChunkPage(context.Background(), "", nil, bounds[0], bounds[1]); err == nil {
			t.Errorf("memory bounds %v were accepted", bounds)
		}
		if _, err := repository.ListHistoricalMemoryCandidatePage(context.Background(), 0, "", bounds[0], bounds[1]); err == nil {
			t.Errorf("candidate bounds %v were accepted", bounds)
		}
	}
	if _, err := repository.ListHistoricalMemoryCandidatePage(context.Background(), -1, "", 10, 0); err == nil {
		t.Fatal("negative candidate job ID was accepted")
	}
	if _, err := repository.ListMemoryChunkPage(context.Background(), " invented ", nil, 10, 0); err == nil {
		t.Fatal("noncanonical memory kind was accepted")
	}
	if _, err := repository.ListHistoricalMemoryCandidatePage(context.Background(), 0, "invented", 10, 0); err == nil {
		t.Fatal("unsupported candidate status was accepted")
	}
}

func TestBoundedChatUIPageUsesOneExactLookaheadRecord(t *testing.T) {
	t.Parallel()
	items, next, hasMore := boundedChatUIPage([]int{1, 2, 3}, 2, 4)
	if !hasMore || next == nil || *next != 6 || len(items) != 2 || items[1] != 2 {
		t.Fatalf("items=%v next=%v has_more=%v", items, next, hasMore)
	}
	items, next, hasMore = boundedChatUIPage([]int{1, 2}, 2, 4)
	if hasMore || next != nil || len(items) != 2 {
		t.Fatalf("terminal items=%v next=%v has_more=%v", items, next, hasMore)
	}
}
