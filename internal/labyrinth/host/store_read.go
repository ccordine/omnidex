package host

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadEpisodeRow(
	ctx context.Context,
	query rowQuerier,
	schema string,
	episode cognition.EpisodeRef,
	locked bool,
) (storedEpisode, error) {
	statement := `
		SELECT scenario_id,scenario_sha256,start_transition,start_transition_sha256,
		       current_number,current_sha256,terminal,receipt_count
		FROM ` + qualifiedHostRelation(schema, "episodes") + ` WHERE episode_id=$1`
	if locked {
		statement += ` FOR UPDATE`
	}
	var (
		scenario              cognition.ScenarioRef
		startRaw, startDigest []byte
		currentNumber         int64
		currentSHA            string
		terminal              bool
		receiptCount          int64
	)
	if err := query.QueryRow(ctx, statement, episode.ID).Scan(
		&scenario.ID, &scenario.SHA256, &startRaw, &startDigest,
		&currentNumber, &currentSHA, &terminal, &receiptCount,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return storedEpisode{}, ErrEpisodeNotFound
		}
		return storedEpisode{}, fmt.Errorf("load labyrinth durable episode: %w", err)
	}
	if currentNumber < 1 || receiptCount < 0 {
		return storedEpisode{}, fmt.Errorf("%w: invalid episode counters", ErrReceiptCorrupt)
	}
	var start cognition.Transition
	if err := decodeExact(startRaw, string(startDigest), &start); err != nil {
		return storedEpisode{}, err
	}
	current := cognition.WorldRevision{
		EpisodeID: episode.ID, Number: uint64(currentNumber), SHA256: currentSHA,
	}
	if err := scenario.Validate(); err != nil {
		return storedEpisode{}, fmt.Errorf("%w: scenario: %v", ErrReceiptCorrupt, err)
	}
	if err := start.ValidateStart(); err != nil || start.Current.EpisodeID != episode.ID {
		return storedEpisode{}, fmt.Errorf("%w: invalid start transition: %v", ErrReceiptCorrupt, err)
	}
	if err := current.Validate(); err != nil || current.Number < start.Current.Number {
		return storedEpisode{}, fmt.Errorf("%w: invalid current revision: %v", ErrReceiptCorrupt, err)
	}
	return storedEpisode{
		Episode: episode, Scenario: scenario, Start: start, Current: current,
		Terminal: terminal, ReceiptCount: receiptCount,
	}, nil
}

func loadActionRow(
	ctx context.Context,
	query rowQuerier,
	schema string,
	episode cognition.EpisodeRef,
	actionID cognition.ActionID,
) (storedAction, bool, error) {
	row := query.QueryRow(ctx, `
		SELECT action_id,request_sha256,expected_number,expected_sha256,
		       action_json,action_json_sha256,
		       actor_job_id,actor_generation,actor_step_id,actor_attempt,actor_worker_id,
		       outcome,result_number,result_sha256,
		       transition_json,transition_json_sha256,failure_json,failure_json_sha256
		FROM `+qualifiedHostRelation(schema, "action_receipts")+`
		WHERE episode_id=$1 AND action_id=$2
	`, episode.ID, actionID)
	return scanActionRow(row, episode)
}

func scanActionRow(row pgx.Row, episode cognition.EpisodeRef) (storedAction, bool, error) {
	var (
		actionID                                 cognition.ActionID
		requestDigest, expectedSHA, actionDigest string
		expectedNumber                           int64
		actionRaw                                []byte
		actorJob, actorGeneration, actorStep     int64
		actorAttempt                             int64
		actorWorker                              string
		outcome                                  string
		resultNumber                             *int64
		resultSHA                                *string
		transitionRaw, failureRaw                []byte
		transitionDigest, failureDigest          *string
	)
	if err := row.Scan(
		&actionID, &requestDigest, &expectedNumber, &expectedSHA, &actionRaw, &actionDigest,
		&actorJob, &actorGeneration, &actorStep, &actorAttempt, &actorWorker,
		&outcome, &resultNumber, &resultSHA, &transitionRaw, &transitionDigest,
		&failureRaw, &failureDigest,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return storedAction{}, false, nil
		}
		return storedAction{}, false, fmt.Errorf("load labyrinth durable action receipt: %w", err)
	}
	if expectedNumber < 1 {
		return storedAction{}, false, fmt.Errorf("%w: invalid expected revision number", ErrReceiptCorrupt)
	}
	var action cognition.RegisteredAction
	if err := decodeExact(actionRaw, actionDigest, &action); err != nil {
		return storedAction{}, false, err
	}
	expected := cognition.WorldRevision{
		EpisodeID: episode.ID, Number: uint64(expectedNumber), SHA256: expectedSHA,
	}
	if err := expected.Validate(); err != nil || action.ID == "" || action.ID != actionID || action.Actor.Validate() != nil ||
		action.Actor.JobID != actorJob || action.Actor.Generation != actorGeneration ||
		action.Actor.StepID != actorStep || actorAttempt < 1 || action.Actor.Attempt != uint64(actorAttempt) ||
		action.Actor.WorkerID != actorWorker {
		return storedAction{}, false, fmt.Errorf("%w: invalid action authority", ErrReceiptCorrupt)
	}
	actualRequestDigest, err := actionRequestSHA256(action)
	if err != nil || actualRequestDigest != requestDigest {
		return storedAction{}, false, fmt.Errorf("%w: action request digest mismatch", ErrReceiptCorrupt)
	}
	receipt := ActionReceipt{
		Episode: episode, Action: action, Expected: expected, RequestSHA256: requestDigest,
	}
	stored := storedAction{Receipt: receipt}
	switch outcome {
	case "transition":
		if resultNumber == nil || *resultNumber < 2 || resultSHA == nil || transitionDigest == nil {
			return storedAction{}, false, fmt.Errorf("%w: incomplete transition receipt", ErrReceiptCorrupt)
		}
		var transition cognition.Transition
		if err := decodeExact(transitionRaw, *transitionDigest, &transition); err != nil {
			return storedAction{}, false, err
		}
		if err := transition.ValidateApply(episode, expected, action); err != nil ||
			transition.Current.Number != uint64(*resultNumber) || transition.Current.SHA256 != *resultSHA {
			return storedAction{}, false, fmt.Errorf("%w: invalid transition receipt: %v", ErrReceiptCorrupt, err)
		}
		number := uint64(*resultNumber)
		stored.ResultNumber = &number
		receipt.Transition = pointerTransition(transition)
		stored.Receipt = receipt
	case "failure":
		if failureDigest == nil {
			return storedAction{}, false, fmt.Errorf("%w: incomplete failure receipt", ErrReceiptCorrupt)
		}
		var failure cognition.ActionFailure
		if err := decodeExact(failureRaw, *failureDigest, &failure); err != nil {
			return storedAction{}, false, err
		}
		if err := failure.Validate(action, expected); err != nil {
			return storedAction{}, false, fmt.Errorf("%w: invalid failure receipt: %v", ErrReceiptCorrupt, err)
		}
		receipt.Failure = pointerFailure(failure)
		stored.Receipt = receipt
	default:
		return storedAction{}, false, fmt.Errorf("%w: unknown receipt outcome %q", ErrReceiptCorrupt, outcome)
	}
	return stored, true, nil
}

func databaseInt(value uint64, field string) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("%w: %s exceeds PostgreSQL bigint", ErrReceiptCorrupt, field)
	}
	return int64(value), nil
}

func pointerTransition(value cognition.Transition) *cognition.Transition {
	cloned := value.Clone()
	return &cloned
}

func pointerFailure(value cognition.ActionFailure) *cognition.ActionFailure {
	cloned := value.Clone()
	return &cloned
}
