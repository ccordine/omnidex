package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
)

var errCodingLedgerReviewPending = errors.New("coding ledger is waiting for owner review")

func (runtime *nativeRuntimeV3) runCodingLedgerPlanningAction() error {
	configuration, err := codingagent.ConfigurationFromMetadata(runtime.claim.Job.Metadata)
	if err != nil {
		return err
	}
	if configuration.Mode != codingagent.Agents {
		return fmt.Errorf("coding planning requires the PostgreSQL task ledger")
	}
	return runtime.runAgentPlanningAction(configuration)
}

func (runtime *nativeRuntimeV3) runCodingLedgerExecutionAction() error {
	configuration, err := codingagent.ConfigurationFromMetadata(runtime.claim.Job.Metadata)
	if err != nil {
		return err
	}
	if configuration.Mode != codingagent.Agents {
		return fmt.Errorf("coding execution requires the PostgreSQL task ledger")
	}
	return runtime.runAgentCodingAction(configuration)
}
