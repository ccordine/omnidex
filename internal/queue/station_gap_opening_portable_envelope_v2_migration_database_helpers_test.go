package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func stationGapOpeningConstraintCatalog(
	t *testing.T,
	pool *pgxpool.Pool,
	excluded ...string,
) map[string]string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT conname,contype::text,convalidated,
		       encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
		FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass
		  AND NOT (conname=ANY($1))
	`, excluded)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var name, constraintType, digest string
		var validated bool
		if err := rows.Scan(&name, &constraintType, &validated, &digest); err != nil {
			t.Fatal(err)
		}
		result[name] = fmt.Sprintf("%s/%t/%s", constraintType, validated, digest)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func stationGapOpeningConstraintHashes(
	t *testing.T,
	pool *pgxpool.Pool,
	names []string,
) map[string]string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT conname,convalidated,
		       encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
		FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass AND conname=ANY($1)
	`, names)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := make(map[string]string, len(names))
	for rows.Next() {
		var name, digest string
		var validated bool
		if err := rows.Scan(&name, &validated, &digest); err != nil {
			t.Fatal(err)
		}
		if !validated {
			t.Fatalf("station-gap constraint %q is not validated", name)
		}
		result[name] = digest
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func stationGapOpeningConstraintExists(
	t *testing.T,
	pool *pgxpool.Pool,
	name string,
) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conrelid='station_gap_openings'::regclass AND conname=$1
		)
	`, name).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	return exists
}

func stationGapOpeningMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func stationGapOpeningEnvelopeV2Probe(
	t *testing.T,
	sequence int,
	projected bool,
) StationGapOpening {
	t.Helper()
	var job assemblyline.PortableJob
	var err error
	if projected {
		job, err = assemblyline.NewSourceProjectedFragmentCorrectionJob(
			assemblyline.FragmentCorrectionInput{
				CurrentDeclaration: fmt.Sprintf(
					"func Value%d() int { return missing() }", sequence,
				),
				RepairGuidance: "Replace the missing call with one local expression.",
			},
			"go",
		)
	} else {
		job, err = assemblyline.NewConversationResponseJob(
			assemblyline.ConversationResponseInput{
				Kind: assemblyline.ObjectiveKindAnswer,
				ExactInstruction: fmt.Sprintf(
					"Return exact station-gap envelope probe %d.", sequence,
				),
			},
		)
	}
	if err != nil {
		t.Fatal(err)
	}
	owner, err := stationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	const contextTokens = 32768
	opening, err := validateStationGapOpening(StationGapOpenRecord{
		Authority: model.StepAttemptAuthority{
			JobID: int64(1000 + sequence), Generation: 1, StepID: int64(2000 + sequence),
			Attempt: 1, WorkerID: fmt.Sprintf("station-gap-envelope-v2-%d", sequence),
		},
		Job: job, Station: owner, ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(t, job, contextTokens),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	return opening
}

func stationGapOpeningEnvelopeV2NullField(
	t *testing.T,
	opening StationGapOpening,
	field string,
) StationGapOpening {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(opening.PortableEnvelope), &envelope); err != nil {
		t.Fatal(err)
	}
	envelope[field] = json.RawMessage("null")
	encoded, err := exactjson.Canonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	opening.PortableEnvelope = string(encoded)
	opening.PortableEnvelopeSHA256 = stationGapSHA256(opening.PortableEnvelope)
	return opening
}

func insertStationGapOpeningEnvelopeV2Probe(
	t *testing.T,
	pool *pgxpool.Pool,
	opening StationGapOpening,
) error {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
		INSERT INTO station_gap_opening_envelope_v2_probe (
			job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,response_schema,
			projection_envelope,projection_sha256,context_tokens,max_output_tokens,output_limit_mode
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23
		)
	`, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.GapID, opening.Station, opening.Scope, opening.PortableSchema,
		opening.WorkID, opening.WorkKind, opening.PortablePayload, opening.PortablePayloadSHA256,
		opening.PortableEnvelope, opening.PortableEnvelopeSHA256, opening.RendererVersion,
		opening.Prompt, string(opening.ResponseSchema), opening.ProjectionEnvelope,
		opening.ProjectionSHA256, opening.ContextTokens, opening.MaxOutputTokens,
		opening.OutputLimitMode)
	return err
}

func requireStationGapOpeningEnvelopeCheckViolation(t *testing.T, err error) {
	t.Helper()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23514" {
		t.Fatalf("station-gap envelope constraint error=%v, want SQLSTATE 23514", err)
	}
}
