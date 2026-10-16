package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestCodingLedgerRevisionResponseMapsRepositoryStateIntoTheSharedContract(t *testing.T) {
	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	value := queue.CodingLedgerRevision{
		LedgerID: 7, JobID: 11, Generation: 2, Revision: 4, BaseRevision: 3,
		OperationID: "operation", State: "proposed", CoveragePassed: true,
		OwnerTurnDispositions: []model.OwnerStatementDisposition{{Kind: model.OwnerTurnRepresented}},
		LatestDelta: queue.CodingLedgerDiff{
			LedgerID: 7, FromRevision: 3, ToRevision: 4,
			Added: []model.LedgerItemID{"TASK-1"}, Limit: 25, Offset: 50, HasMore: true,
		},
		CreatedAt: now, UpdatedAt: now,
		Ledger: model.EngineeringLedger{ID: "ledger-7", Revision: 4},
		Routes: []queue.CodingLedgerRoute{{Role: "executor", ConfigurationRevision: 4,
			Provider: "codex", Model: "model", ReasoningEffort: "low", ServiceTier: "default"}},
	}

	response := codingLedgerRevisionResponse(value)
	if response.LedgerID != value.LedgerID || response.Revision != value.Revision ||
		response.LatestDelta.FromRevision != 3 || len(response.LatestDelta.Added) != 1 ||
		len(response.Routes) != 1 || response.Routes[0].Role != "executor" {
		t.Fatalf("transport response lost repository authority: %+v", response)
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"limit"`, `"offset"`, `"has_more"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("repository pagination leaked into current-ledger transport: %s", raw)
		}
	}
	value.LatestDelta.Added[0] = "TASK-CHANGED"
	value.Routes[0].Role = "changed"
	if response.LatestDelta.Added[0] != "TASK-1" || response.Routes[0].Role != "executor" {
		t.Fatal("transport response retained mutable repository slices")
	}
}
