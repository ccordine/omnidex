package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const cognitionTraceSchemaAuthorityV1 = "omnidex.cognition-trace-schema-authority.v1"

type cognitionTraceSchemaAuthority struct {
	Schema                 string   `json:"schema"`
	TraceSchema            string   `json:"trace_schema"`
	PageSchema             string   `json:"page_schema"`
	MandatoryRevisionKinds []string `json:"mandatory_revision_kinds"`
}

func requireCognitionTraceSchemaAuthorityTx(ctx context.Context, tx pgx.Tx) error {
	want := cognitionTraceSchemaAuthority{
		Schema:                 cognitionTraceSchemaAuthorityV1,
		TraceSchema:            cognitionTraceAuthoritySchemaV2,
		PageSchema:             CognitionSealedTraceSchemaV2,
		MandatoryRevisionKinds: []string{"belief_revision", "plan_revision"},
	}
	wantRaw, wantSHA, err := cognitionJSON(want)
	if err != nil {
		return err
	}
	var raw []byte
	var persistedSHA string
	if err := tx.QueryRow(ctx, `
		SELECT authority_json,authority_sha256
		FROM cognition_trace_schema_authority WHERE singleton=TRUE
	`).Scan(&raw, &persistedSHA); err != nil {
		return fmt.Errorf("%w: load cognition trace schema authority: %v", ErrCognitionConflict, err)
	}
	var actual cognitionTraceSchemaAuthority
	if !json.Valid(raw) || json.Unmarshal(raw, &actual) != nil ||
		persistedSHA != wantSHA || !bytes.Equal(raw, wantRaw) {
		return fmt.Errorf("%w: cognition trace schema authority changed", ErrCognitionConflict)
	}
	return nil
}
