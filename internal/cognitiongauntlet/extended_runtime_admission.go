package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func admitExtendedRuntime(
	ctx context.Context,
	generated labyrinth.ExtendedCase,
	request ExtendedRuntimeRunRequest,
	authority PairedRunAuthority,
	episode cognition.EpisodeRef,
	run cognitionruntime.RunResult,
	trace productionTrace,
	prerequisiteSHA256 string,
) (ExtendedRuntimeReceipt, error) {
	oracle := generated.PrivateOracle()
	if trace.Header.Seal.Outcome != queue.CognitionEpisodeCompleted ||
		trace.Header.Seal.FinalRevision != run.Terminal.Revision ||
		trace.Header.TraceSHA256 != trace.Header.Seal.TraceSHA256 {
		return ExtendedRuntimeReceipt{}, fmt.Errorf("extended runtime terminal trace authority differs")
	}
	actualCost, actualActions, failedActions, err := replayExtendedActionTrace(
		ctx, generated.ExecutionScenario(), episode, request.Attempt, trace, true,
	)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	actor := extendedEvaluatorActor(cognition.AttemptRef{
		JobID: request.Attempt.JobID, Generation: request.Attempt.Generation,
		StepID: request.Attempt.StepID, Attempt: uint64(request.Attempt.Attempt),
		WorkerID: request.Attempt.WorkerID,
	})
	if _, err := labyrinth.RunExtendedOracle(ctx, generated,
		cognition.EpisodeRef{ID: cognition.EpisodeID(string(episode.ID) + "-witness")}, actor,
	); err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	if err := labyrinth.VerifyExtendedInvalidRails(ctx, generated,
		cognition.EpisodeRef{ID: cognition.EpisodeID(string(episode.ID) + "-invalid")}, actor,
	); err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	if err := labyrinth.VerifyExtendedOmissionRails(ctx, generated,
		cognition.EpisodeRef{ID: cognition.EpisodeID(string(episode.ID) + "-omission")}, actor,
	); err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	proofs, err := extendedRuntimeProofs(generated, trace)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	revisionSHA := ""
	if authority.Suite == SuiteRevise || authority.Suite == SuiteRogue {
		revisionSHA, err = validateExtendedRevisionTrace(trace)
		if err != nil {
			return ExtendedRuntimeReceipt{}, err
		}
	}
	evidenceClass, err := extendedClientEvidenceClass(request.Client)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	witnessCost := 0
	for _, action := range oracle.Witness {
		witnessCost += action.Cost
	}
	policyCalls, err := acceptedExtendedPolicyCalls(trace)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	receipt := ExtendedRuntimeReceipt{
		Schema: ExtendedRuntimeReceiptSchemaV1, Authority: authority,
		EpisodeID: string(episode.ID), Seal: trace.Header.Seal,
		PolicyCalls: policyCalls, EnvironmentActions: uint32(actualActions),
		FailedActions: uint32(failedActions),
		EvidenceClass: evidenceClass, PromotionEligible: false,
		CostBaseline: extendedCostWitnessOnly, ActualCost: actualCost, WitnessCost: witnessCost,
		Proofs: proofs, RevisionTraceSHA256: revisionSHA,
		PrerequisiteBundleSHA256: prerequisiteSHA256,
	}
	if run.EnvironmentActions != 0 && uint32(actualActions) != run.EnvironmentActions {
		return ExtendedRuntimeReceipt{}, fmt.Errorf("extended runtime metrics differ from replayed actions")
	}
	receipt.ReceiptSHA256, err = extendedRuntimeReceiptSHA(receipt)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	return receipt, receipt.Validate()
}

func acceptedExtendedPolicyCalls(trace productionTrace) (uint32, error) {
	var result uint32
	for _, record := range trace.Records {
		if record.Kind != "policy_result" {
			continue
		}
		var call cognitionpolicy.CallResult
		if err := decodeProductionPayload(record.Payload, &call, "extended policy result"); err != nil {
			return 0, err
		}
		if call.Status != cognitionpolicy.CallResultAccepted {
			return 0, fmt.Errorf("extended runtime contains a non-accepted policy result")
		}
		result++
	}
	if result == 0 {
		return 0, fmt.Errorf("extended runtime trace contains no accepted policy calls")
	}
	return result, nil
}

func replayExtendedActionTrace(
	ctx context.Context,
	scenario labyrinth.Scenario,
	episode cognition.EpisodeRef,
	attempt model.StepAttemptAuthority,
	trace productionTrace,
	requireTerminal bool,
) (int, int, int, error) {
	actions := make([]queue.CognitionTraceAction, 0)
	transitions := make(map[cognition.ActionID]cognition.Transition)
	var start cognition.Transition
	for _, record := range trace.Records {
		switch record.Kind {
		case "action":
			var action queue.CognitionTraceAction
			if err := decodeProductionPayload(record.Payload, &action, "extended action"); err != nil {
				return 0, 0, 0, err
			}
			if err := action.Validate(); err != nil {
				return 0, 0, 0, fmt.Errorf("extended runtime contains an invalid action")
			}
			actions = append(actions, action)
		case "transition":
			var transition cognition.Transition
			if err := decodeProductionPayload(record.Payload, &transition, "extended transition"); err != nil {
				return 0, 0, 0, err
			}
			if transition.ActionID == "" {
				start = transition
			} else {
				transitions[transition.ActionID] = transition
			}
		}
	}
	if start.Current.EpisodeID != episode.ID {
		return 0, 0, 0, fmt.Errorf("extended runtime trace lacks exact action transitions")
	}
	actor := cognition.AttemptRef{
		JobID: attempt.JobID, Generation: attempt.Generation, StepID: attempt.StepID,
		Attempt: uint64(attempt.Attempt), WorkerID: attempt.WorkerID,
	}
	if err := actor.Validate(); err != nil {
		return 0, 0, 0, err
	}
	environment, err := labyrinth.NewEnvironment(
		scenario, episode, func(_ context.Context, candidate cognition.AttemptRef) error {
			if candidate != actor {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
	)
	if err != nil {
		return 0, 0, 0, err
	}
	replayedStart, err := environment.Start(ctx, scenario.Ref())
	if err != nil || !reflect.DeepEqual(replayedStart, start) {
		return 0, 0, 0, fmt.Errorf("extended runtime start transition failed exact replay: %v", err)
	}
	current, cost, failures := replayedStart.Current, 0, 0
	terminal := false
	for index, action := range actions {
		if action.RegisteredAction.Actor != actor {
			return 0, 0, 0, fmt.Errorf("extended runtime action %d changed actor", index+1)
		}
		transition, applyErr := environment.Apply(ctx, episode, current, action.RegisteredAction)
		if action.Status == queue.CognitionActionFailed {
			var failure cognition.ActionFailure
			if !errors.As(applyErr, &failure) || action.Failure == nil ||
				!reflect.DeepEqual(failure, *action.Failure) {
				return 0, 0, 0, fmt.Errorf("extended failed action %d changed its typed pain signal", index+1)
			}
			failures++
			continue
		}
		sealed, exists := transitions[action.RegisteredAction.ID]
		if applyErr != nil || !exists || !reflect.DeepEqual(transition, sealed) {
			return 0, 0, 0, fmt.Errorf("extended runtime action %d failed exact private replay: %v", index+1, applyErr)
		}
		current, cost = transition.Current, cost+transition.Cost
		terminal = transition.Terminal
	}
	if (requireTerminal && !terminal) || len(transitions) != len(actions)-failures {
		return 0, 0, 0, fmt.Errorf("extended runtime replay did not reach the code-owned terminal goal")
	}
	return cost, len(actions), failures, nil
}

func extendedRuntimeProofs(
	generated labyrinth.ExtendedCase,
	trace productionTrace,
) ([]ExtendedRuntimeProof, error) {
	public := generated.PublicArtifact()
	oracle := generated.PrivateOracle()
	separation, err := digestJSON(struct {
		Scenario cognition.ScenarioRef `json:"scenario"`
		Oracle   string                `json:"oracle_sha256"`
	}{public.Scenario, oracle.OracleSHA256})
	if err != nil {
		return nil, err
	}
	rails, err := digestJSON(struct {
		Invalid  []labyrinth.ExtendedInvalidRail  `json:"invalid"`
		Omission []labyrinth.ExtendedOmissionRail `json:"omission"`
	}{oracle.InvalidRails, oracle.OmissionRails})
	if err != nil {
		return nil, err
	}
	runtime, err := digestJSON(struct {
		TraceSHA string `json:"trace_sha256"`
		Records  int    `json:"records"`
	}{trace.Header.TraceSHA256, len(trace.Records)})
	if err != nil {
		return nil, err
	}
	return []ExtendedRuntimeProof{
		{Kind: ProofPublicPrivateSeparation, SHA256: separation},
		{Kind: ProofValidInvalidRails, SHA256: rails},
		{Kind: ProofOrdinaryRuntime, SHA256: runtime},
	}, nil
}
