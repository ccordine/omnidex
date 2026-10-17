package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ledgerapi"
)

func (session *chatSession) redirectLedgerRevision(jobID int64) (int64, error) {
	if session.planReview != nil {
		if session.planReview.snapshot.JobID != jobID {
			return 0, fmt.Errorf("redirect review belongs to a different job")
		}
		return session.planReview.snapshot.Revision, nil
	}
	ledger, err := awaitChatRequest(session.ctx, session.signals,
		func(requestContext context.Context) (ledgerapi.CodingLedgerRevision, error) {
			return session.client.CodingLedger(requestContext, session.channel, session.workspaceIdentity, jobID)
		})
	if client.IsHTTPStatus(err, http.StatusNotFound) {
		// A job can still be producing its first proposal. This is the explicit
		// no-ledger state; server revision validation remains authoritative.
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load redirect ledger revision: %w", err)
	}
	if ledger.Generation != session.active.Job.CurrentGeneration {
		return 0, fmt.Errorf("redirect ledger generation %d differs from active generation %d", ledger.Generation, session.active.Job.CurrentGeneration)
	}
	return ledger.Revision, nil
}
