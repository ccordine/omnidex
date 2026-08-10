package host

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func persistActionReceipt(
	ctx context.Context,
	tx pgx.Tx,
	schema string,
	episode storedEpisode,
	receipt ActionReceipt,
) error {
	actionRaw, actionDigest, err := encodeExact(receipt.Action)
	if err != nil {
		return err
	}
	expectedNumber, err := databaseInt(receipt.Expected.Number, "expected revision")
	if err != nil {
		return err
	}
	actorAttempt, err := databaseInt(receipt.Action.Actor.Attempt, "actor attempt")
	if err != nil {
		return err
	}
	var (
		outcome                         string
		resultNumber                    *int64
		resultSHA                       *string
		transitionRaw, failureRaw       []byte
		transitionDigest, failureDigest *string
	)
	if receipt.Transition != nil && receipt.Failure == nil {
		outcome = "transition"
		number, convertErr := databaseInt(receipt.Transition.Current.Number, "result revision")
		if convertErr != nil {
			return convertErr
		}
		resultNumber = &number
		resultSHA = &receipt.Transition.Current.SHA256
		raw, digest, encodeErr := encodeExact(*receipt.Transition)
		if encodeErr != nil {
			return encodeErr
		}
		transitionRaw, transitionDigest = raw, &digest
	} else if receipt.Failure != nil && receipt.Transition == nil {
		outcome = "failure"
		raw, digest, encodeErr := encodeExact(*receipt.Failure)
		if encodeErr != nil {
			return encodeErr
		}
		failureRaw, failureDigest = raw, &digest
	} else {
		return fmt.Errorf("%w: action receipt must contain exactly one result", ErrReceiptCorrupt)
	}
	receiptNumber := episode.ReceiptCount + 1
	_, err = tx.Exec(ctx, `
		INSERT INTO `+qualifiedHostRelation(schema, "action_receipts")+`(
			episode_id,action_id,receipt_number,request_sha256,
			expected_number,expected_sha256,action_json,action_json_sha256,
			actor_job_id,actor_generation,actor_step_id,actor_attempt,actor_worker_id,
			outcome,result_number,result_sha256,transition_json,transition_json_sha256,
			failure_json,failure_json_sha256
		) VALUES(
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20
		)
	`, receipt.Episode.ID, receipt.Action.ID, receiptNumber, receipt.RequestSHA256,
		expectedNumber, receipt.Expected.SHA256, actionRaw, actionDigest,
		receipt.Action.Actor.JobID, receipt.Action.Actor.Generation, receipt.Action.Actor.StepID,
		actorAttempt, receipt.Action.Actor.WorkerID, outcome, resultNumber, resultSHA,
		transitionRaw, transitionDigest, failureRaw, failureDigest)
	if err != nil {
		return fmt.Errorf("persist Labyrinth action receipt: %w", err)
	}
	current := episode.Current
	terminal := episode.Terminal
	if receipt.Transition != nil {
		current = receipt.Transition.Current
		terminal = receipt.Transition.Terminal
	}
	result, err := tx.Exec(ctx, `
		UPDATE `+qualifiedHostRelation(schema, "episodes")+`
		SET current_number=$2,current_sha256=$3,terminal=$4,
		    receipt_count=receipt_count+1,updated_at=clock_timestamp()
		WHERE episode_id=$1 AND current_number=$5 AND current_sha256=$6
		      AND terminal=$7 AND receipt_count=$8
	`, episode.Episode.ID, int64(current.Number), current.SHA256, terminal,
		int64(episode.Current.Number), episode.Current.SHA256, episode.Terminal, episode.ReceiptCount)
	if err != nil {
		return fmt.Errorf("advance Labyrinth durable episode: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: episode head changed inside locked transaction", ErrReceiptCorrupt)
	}
	return nil
}
