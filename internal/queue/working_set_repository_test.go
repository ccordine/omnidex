package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/workingset"
)

func TestWorkingSetRepositoryRejectsInvalidRequestsBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	repository := &Repository{}
	if _, err := repository.CreateCurrentWorkingSet(
		context.Background(), 1, 1, workingset.Budget{MaxItems: 1, MaxBytes: 1},
	); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured creation error=%v", err)
	}
	if _, err := repository.CurrentWorkingSet(context.Background(), 0); err == nil ||
		!strings.Contains(err.Error(), "job ID") {
		t.Fatalf("invalid current read error=%v", err)
	}
	if _, err := repository.WorkingSetForGeneration(context.Background(), 1, 0); err == nil ||
		!strings.Contains(err.Error(), "generation") {
		t.Fatalf("invalid generation read error=%v", err)
	}
	if _, err := repository.ApplyWorkingSetCommand(context.Background(), 1, 0, nil); err == nil ||
		!strings.Contains(err.Error(), "generation") {
		t.Fatalf("invalid apply error=%v", err)
	}
	if _, err := repository.ListWorkingSetEvents(context.Background(), 1, 1, -1, 1); err == nil ||
		!strings.Contains(err.Error(), "cursor") {
		t.Fatalf("negative cursor error=%v", err)
	}
	for _, limit := range []int{0, maxWorkingSetEventPageSize + 1} {
		if _, err := repository.ListWorkingSetEvents(context.Background(), 1, 1, 0, limit); err == nil ||
			!strings.Contains(err.Error(), "page limit") {
			t.Fatalf("invalid page limit %d error=%v", limit, err)
		}
	}
}
