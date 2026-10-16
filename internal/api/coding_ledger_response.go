package api

import (
	"github.com/gryph/omnidex/internal/ledgerapi"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func codingLedgerRevisionResponse(value queue.CodingLedgerRevision) ledgerapi.CodingLedgerRevision {
	routes := make([]ledgerapi.CodingLedgerRoute, len(value.Routes))
	for index, route := range value.Routes {
		routes[index] = ledgerapi.CodingLedgerRoute{
			Role: route.Role, ConfigurationRevision: route.ConfigurationRevision,
			Provider: route.Provider, Model: route.Model,
			ReasoningEffort: route.ReasoningEffort, ServiceTier: route.ServiceTier,
		}
	}
	return ledgerapi.CodingLedgerRevision{
		LedgerID: value.LedgerID, JobID: value.JobID, Generation: value.Generation,
		Revision: value.Revision, BaseRevision: value.BaseRevision,
		OperationID: value.OperationID, State: value.State,
		OwnerTurnID:           value.OwnerTurnID,
		OwnerTurnDispositions: append([]model.OwnerStatementDisposition(nil), value.OwnerTurnDispositions...),
		LatestDelta: ledgerapi.CodingLedgerDelta{
			LedgerID:     value.LatestDelta.LedgerID,
			FromRevision: value.LatestDelta.FromRevision, ToRevision: value.LatestDelta.ToRevision,
			Added:    append([]model.LedgerItemID(nil), value.LatestDelta.Added...),
			Amended:  append([]model.LedgerItemID(nil), value.LatestDelta.Amended...),
			Removed:  append([]model.LedgerItemID(nil), value.LatestDelta.Removed...),
			Restored: append([]model.LedgerItemID(nil), value.LatestDelta.Restored...),
		},
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		Ledger: value.Ledger, Routes: routes, Applied: value.Applied,
	}
}
