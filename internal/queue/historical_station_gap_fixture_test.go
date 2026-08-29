package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

const historicalPortableRendererV4 = "omnidex.render-portable-job.v4"

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
