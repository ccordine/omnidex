package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
)

type codingLedgerRequest struct {
	Instruction            string
	Feedback               []string
	PlanningOperationID    string
	ObservedLedgerRevision int64
}

func (request codingLedgerRequest) authority() string {
	return strings.Join(append([]string{request.Instruction}, request.Feedback...), "\n")
}

func (runtime *nativeRuntimeV3) codingLedgerRequest() (codingLedgerRequest, error) {
	if runtime == nil || runtime.claim == nil {
		return codingLedgerRequest{}, fmt.Errorf("coding ledger requires a claimed job")
	}
	instruction := runtime.claim.Job.Instruction
	if runtime.claim.Job.Pipeline == model.PipelineScrum {
		metadata, err := scrum.DecodeStoredJobMetadata(runtime.claim.Job.Metadata)
		if err != nil {
			return codingLedgerRequest{}, err
		}
		cardLines, err := scrum.ContextLinesFromMetadata(runtime.claim.Job.Metadata)
		if err != nil {
			return codingLedgerRequest{}, err
		}
		card := strings.TrimSpace(strings.Join(cardLines, "\n"))
		if card == "" {
			return codingLedgerRequest{}, fmt.Errorf("Scrum coding requires authoritative card context")
		}
		if !metadata.ChannelOrigin {
			instruction = card
		} else {
			instruction = strings.TrimSpace(strings.Join([]string{instruction, card}, "\n"))
		}
	}
	if strings.TrimSpace(instruction) == "" {
		return codingLedgerRequest{}, fmt.Errorf("coding ledger requires a non-empty current instruction")
	}
	request := codingLedgerRequest{Instruction: instruction}
	continuity, err := runtime.svc.repo.ObjectiveContinuityAuthorities(
		runtime.ctx, runtime.claim.Job, "v3_coding_plan",
	)
	if err != nil {
		return codingLedgerRequest{}, err
	}
	request.Feedback, err = continuity.CodingFeedback()
	if err != nil {
		return codingLedgerRequest{}, err
	}
	request.PlanningOperationID = string(continuity.ReplanOperationID)
	request.ObservedLedgerRevision = continuity.ObservedLedgerRevision
	return request, nil
}
