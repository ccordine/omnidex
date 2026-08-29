package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	historicalPortableRendererV4 = "omnidex.render-portable-job.v4"
	historicalPortableRendererV5 = "omnidex.render-portable-job.v5"
)

func historicalStationGapResponseSchema(
	renderer string,
) (json.RawMessage, error) {
	switch renderer {
	case "omnidex.render-portable-job.v1",
		"omnidex.render-portable-job.v2",
		"omnidex.render-portable-job.v3":
		return json.RawMessage(`{}`), nil
	case historicalPortableRendererV4:
		return json.RawMessage(`null`), nil
	default:
		return nil, fmt.Errorf("historical station fixture has unsupported renderer %q", renderer)
	}
}

func freezeHistoricalRawStationGapV4(
	t testing.TB,
	opening StationGapOpening,
) StationGapOpening {
	t.Helper()
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{
		Prompt: opening.Prompt, Renderer: historicalPortableRendererV4,
		ResponseSchema: json.RawMessage(`null`),
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.RendererVersion = historicalPortableRendererV4
	opening.ProjectionEnvelope = string(projection)
	opening.ProjectionSHA256 = stationGapSHA256(string(projection))
	return opening
}

func freezeHistoricalRawStationGapV5(
	t testing.TB,
	opening StationGapOpening,
) StationGapOpening {
	t.Helper()
	projection, err := exactjson.Canonical(struct {
		Prompt   string `json:"prompt"`
		Renderer string `json:"renderer"`
	}{
		Prompt: opening.Prompt, Renderer: historicalPortableRendererV5,
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.RendererVersion = historicalPortableRendererV5
	opening.ProjectionEnvelope = string(projection)
	opening.ProjectionSHA256 = stationGapSHA256(string(projection))
	return opening
}

func insertHistoricalStationGapOpeningTx(
	ctx context.Context,
	tx pgx.Tx,
	opening *StationGapOpening,
) error {
	if opening == nil {
		return fmt.Errorf("historical station gap fixture requires an opening")
	}
	responseSchema, err := historicalStationGapResponseSchema(opening.RendererVersion)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO station_gap_openings (
			job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,response_schema,
			projection_envelope,projection_sha256,context_tokens,max_output_tokens,output_limit_mode
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23
		)
		RETURNING id,created_at
	`, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.GapID, opening.Station, opening.Scope, opening.PortableSchema,
		opening.WorkID, opening.WorkKind, opening.PortablePayload, opening.PortablePayloadSHA256,
		opening.PortableEnvelope, opening.PortableEnvelopeSHA256, opening.RendererVersion,
		opening.Prompt, string(responseSchema), opening.ProjectionEnvelope,
		opening.ProjectionSHA256, opening.ContextTokens, opening.MaxOutputTokens,
		opening.OutputLimitMode).Scan(&opening.ID, &opening.CreatedAt)
	if err != nil {
		return fmt.Errorf("persist historical station gap fixture: %w", err)
	}
	return nil
}

func completeHistoricalStationGapWithDiscoveryFailure(
	t *testing.T,
	pool *pgxpool.Pool,
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	errorText string,
) {
	t.Helper()
	selection := llm.ProviderIdentitySelection{
		Model: "qwen:9b", NativeContextLimit: gap.ContextTokens,
	}
	discovery, err := historicalStationDiscoveryOpening(authority, gap, selection)
	if err != nil {
		t.Fatal(err)
	}
	failure := stationCallIdentityFailure(t, llm.PreparedModel{
		ContextModel: selection.Model, ContextTokens: selection.NativeContextLimit,
	})
	record := StationDiscoveryReceiptRecord{
		Authority: authority, GapID: gap.GapID,
		Observed: llm.ObservedProviderIdentity{
			Evidence: failure.ProviderIdentityEvidence,
		},
		FailureReason: StationDiscoveryFailureEvidenceRejected,
		Error:         "historical exact provider discovery failed",
	}
	terminal := StationGapTerminalRecord{
		Authority: authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: errorText,
	}
	if err := validateStationGapTerminal(terminal); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if err := requireRunningStationAttemptTx(t.Context(), tx, authority); err != nil {
		t.Fatal(err)
	}
	if err := requireHistoricalStationGapClosingAuthorityTx(
		t.Context(), tx, terminal,
	); err != nil {
		t.Fatal(err)
	}
	if err := insertStationDiscoveryOpeningTx(t.Context(), tx, &discovery); err != nil {
		t.Fatal(err)
	}
	record.OpeningID = discovery.ID
	if _, err := insertStationDiscoveryReceiptTx(
		t.Context(), tx, record, discovery,
	); err != nil {
		t.Fatal(err)
	}
	var outcome StationGapOutcome
	if err := insertStationGapOutcomeTx(t.Context(), tx, terminal, &outcome); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func historicalStationDiscoveryOpening(
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
	selection llm.ProviderIdentitySelection,
) (StationDiscoveryOpening, error) {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return StationDiscoveryOpening{}, err
	}
	if err := selection.Validate(); err != nil {
		return StationDiscoveryOpening{}, err
	}
	if gap.ID < 1 || gap.JobID != authority.JobID ||
		gap.Generation != authority.Generation || gap.StepID != authority.StepID ||
		gap.StepAttempt != authority.Attempt || gap.WorkerID != authority.WorkerID ||
		selection.NativeContextLimit != gap.ContextTokens {
		return StationDiscoveryOpening{}, fmt.Errorf(
			"historical station discovery differs from its exact gap authority",
		)
	}
	selectionJSON, err := exactjson.Canonical(selection)
	if err != nil {
		return StationDiscoveryOpening{}, err
	}
	challenge, err := llm.DeriveProviderIdentityDiscoveryChallenge(
		"station-gap:"+gap.GapID, selection,
	)
	if err != nil {
		return StationDiscoveryOpening{}, err
	}
	return StationDiscoveryOpening{
		GapOpeningID: gap.ID, JobID: authority.JobID, Generation: authority.Generation,
		StepID: authority.StepID, StepAttempt: authority.Attempt,
		WorkerID: authority.WorkerID, GapID: gap.GapID,
		Selection:       append(json.RawMessage(nil), selectionJSON...),
		SelectionSHA256: stationGapSHA256(string(selectionJSON)), Challenge: challenge,
	}, nil
}

func requireHistoricalStationGapClosingAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	record StationGapTerminalRecord,
) error {
	var jobID, generation, stepID, stepAttempt int64
	var workerID, gapID string
	if err := tx.QueryRow(ctx, `
		SELECT job_id,generation,step_id,step_attempt,worker_id,gap_id
		FROM station_gap_openings WHERE id=$1 FOR SHARE
	`, record.OpeningID).Scan(
		&jobID, &generation, &stepID, &stepAttempt, &workerID, &gapID,
	); err != nil {
		return err
	}
	if jobID != record.Authority.JobID || generation != record.Authority.Generation ||
		stepID != record.Authority.StepID || stepAttempt != record.Authority.Attempt ||
		workerID != record.Authority.WorkerID || gapID != record.GapID {
		return fmt.Errorf(
			"historical station gap outcome differs from its exact original attempt",
		)
	}
	return nil
}
