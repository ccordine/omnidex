package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5"
)

const (
	CognitionTraceKindProviderBrainBootstrap = "provider_brain_bootstrap"
	CognitionBrainBootstrapTraceSchemaV1     = "omnidex.provider-brain-bootstrap-trace.v1"
)

type CognitionBrainBootstrapTraceSource string

const (
	CognitionBrainBootstrapEpisodeStart      CognitionBrainBootstrapTraceSource = "episode_start"
	CognitionBrainBootstrapEpisodeReplay     CognitionBrainBootstrapTraceSource = "episode_replay"
	CognitionBrainBootstrapActivationFailure CognitionBrainBootstrapTraceSource = "activation_failure"
)

type CognitionBrainBootstrapTrace struct {
	Schema     string                             `json:"schema"`
	Source     CognitionBrainBootstrapTraceSource `json:"source"`
	SourceID   string                             `json:"source_id"`
	EpisodeID  cognition.EpisodeID                `json:"episode_id"`
	Actor      cognition.AttemptRef               `json:"actor"`
	Brain      cognitionpolicy.AttestedBrain      `json:"brain"`
	Evidence   llm.ProviderIdentityEvidenceRef    `json:"evidence"`
	RecordedAt time.Time                          `json:"recorded_at"`
}

func (value CognitionBrainBootstrapTrace) Validate() error {
	if value.Schema != CognitionBrainBootstrapTraceSchemaV1 ||
		cognitionEpisodeIdentityValid(value.EpisodeID) != nil || value.Actor.Validate() != nil ||
		value.Brain.Validate() != nil || value.Evidence.Validate() != nil ||
		value.Evidence != value.Brain.BootstrapObservation.Evidence ||
		value.RecordedAt.IsZero() || value.RecordedAt.Location() != time.UTC ||
		value.RecordedAt.Nanosecond()%1_000 != 0 {
		return fmt.Errorf("%w: provider Brain bootstrap trace is invalid", ErrCognitionConflict)
	}
	switch value.Source {
	case CognitionBrainBootstrapEpisodeStart:
		if value.SourceID != value.Evidence.ID {
			return fmt.Errorf("%w: episode bootstrap trace source changed", ErrCognitionConflict)
		}
	case CognitionBrainBootstrapEpisodeReplay:
		if !cognitionDigestIdentity(value.SourceID, "cognition_replay_bootstrap_") {
			return fmt.Errorf("%w: replay bootstrap trace source changed", ErrCognitionConflict)
		}
	case CognitionBrainBootstrapActivationFailure:
		if !cognitionDigestIdentity(value.SourceID, "cognition_provider_failure_") {
			return fmt.Errorf("%w: failed activation bootstrap trace source changed", ErrCognitionConflict)
		}
	default:
		return fmt.Errorf("%w: provider Brain bootstrap trace source is not registered", ErrCognitionConflict)
	}
	return nil
}

const cognitionBrainBootstrapTraceRows = `WITH bootstrap_sources AS (
	SELECT 'episode_start'::text AS source,evidence.evidence_id AS source_id,
	       episode.episode_id,episode.job_id,episode.generation,episode.step_id,
	       episode.created_attempt AS attempt,episode.created_worker_id AS worker_id,
	       episode.attested_brain_json AS brain_json,NULL::text AS observation_json,
	       evidence.evidence_id,episode.created_at AS recorded_at,0::bigint AS sequence,1 AS phase
	FROM cognition_episode_provider_identity_evidence evidence
	JOIN cognition_episodes episode ON episode.episode_id=evidence.episode_id
	UNION ALL
	SELECT 'episode_replay',replay.replay_id,replay.episode_id,replay.job_id,replay.generation,
	       replay.step_id,replay.step_attempt,replay.worker_id,episode.attested_brain_json,
	       replay.provider_observation_json,replay.evidence_id,replay.created_at,
	       replay.step_attempt,2
	FROM cognition_episode_replay_provider_identity_evidence replay
	JOIN cognition_episodes episode ON episode.episode_id=replay.episode_id
	UNION ALL
	SELECT 'activation_failure',failure.record_id,failure.episode_id,failure.job_id,
	       failure.generation,failure.step_id,failure.step_attempt,failure.worker_id,
	       failure.bootstrap_brain_json,NULL::text,failure.bootstrap_evidence_id,
	       failure.created_at,failure.record_number,3
	FROM cognition_provider_activation_failures failure
	WHERE failure.failure_kind='provider_process'
) SELECT source,source_id,episode_id,job_id,generation,step_id,attempt,worker_id,
	brain_json,observation_json,evidence_id,recorded_at,sequence,phase FROM bootstrap_sources`

type persistedCognitionBrainBootstrapTrace struct {
	value           CognitionBrainBootstrapTrace
	brainJSON       []byte
	observationJSON []byte
	evidenceID      string
	sequence        int64
	phase           int
}

func appendCognitionBrainBootstrapTraceRecordsTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeID,
	records []cognitionTraceRecord,
) ([]cognitionTraceRecord, error) {
	rows, err := tx.Query(ctx, cognitionBrainBootstrapTraceRows+
		` WHERE episode_id=$1 ORDER BY phase,sequence,source_id`, episode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		persisted, err := scanCognitionBrainBootstrapTrace(rows)
		if err != nil {
			return nil, err
		}
		raw, err := restoreCognitionBrainBootstrapTrace(persisted)
		if err != nil {
			return nil, err
		}
		records = append(records, cognitionTraceRecord{
			Kind:  CognitionTraceKindProviderBrainBootstrap,
			Phase: persisted.phase, Sequence: persisted.sequence,
			ID: persisted.value.SourceID, SHA256: cognitionPayloadSHA(raw),
		})
	}
	return records, rows.Err()
}

type cognitionBootstrapTraceScanner interface {
	Scan(...any) error
}

func scanCognitionBrainBootstrapTrace(
	scanner cognitionBootstrapTraceScanner,
) (persistedCognitionBrainBootstrapTrace, error) {
	value := persistedCognitionBrainBootstrapTrace{}
	var observation *string
	if err := scanner.Scan(
		&value.value.Source, &value.value.SourceID, &value.value.EpisodeID,
		&value.value.Actor.JobID, &value.value.Actor.Generation, &value.value.Actor.StepID,
		&value.value.Actor.Attempt, &value.value.Actor.WorkerID, &value.brainJSON,
		&observation, &value.evidenceID, &value.value.RecordedAt,
		&value.sequence, &value.phase,
	); err != nil {
		return value, err
	}
	if observation != nil {
		value.observationJSON = []byte(*observation)
	}
	value.value.RecordedAt = value.value.RecordedAt.UTC()
	return value, nil
}

func restoreCognitionBrainBootstrapTrace(
	persisted persistedCognitionBrainBootstrapTrace,
) ([]byte, error) {
	value := &persisted.value
	value.Schema = CognitionBrainBootstrapTraceSchemaV1
	if err := cognitionDecodeExact(persisted.brainJSON, &value.Brain); err != nil {
		return nil, err
	}
	if len(persisted.observationJSON) > 0 {
		if err := cognitionDecodeExact(
			persisted.observationJSON, &value.Brain.BootstrapObservation,
		); err != nil {
			return nil, err
		}
	}
	value.Evidence = value.Brain.BootstrapObservation.Evidence
	if value.Evidence.ID != persisted.evidenceID || value.Validate() != nil {
		return nil, fmt.Errorf("%w: provider Brain bootstrap trace changed", ErrCognitionConflict)
	}
	raw, err := exactjson.Canonical(value)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func loadCognitionBrainBootstrapTracePayloadTx(
	ctx context.Context,
	tx pgx.Tx,
	episode cognition.EpisodeID,
	record cognitionTraceRecord,
) ([]byte, error) {
	persisted, err := scanCognitionBrainBootstrapTrace(tx.QueryRow(
		ctx, cognitionBrainBootstrapTraceRows+` WHERE episode_id=$1 AND source_id=$2`,
		episode, record.ID,
	))
	if err != nil {
		return nil, err
	}
	raw, err := restoreCognitionBrainBootstrapTrace(persisted)
	if err != nil || cognitionPayloadSHA(raw) != record.SHA256 ||
		persisted.sequence != record.Sequence || persisted.phase != record.Phase {
		return nil, fmt.Errorf("%w: sealed provider Brain bootstrap trace changed", ErrCognitionConflict)
	}
	return raw, nil
}
