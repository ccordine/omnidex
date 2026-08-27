package queue

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func insertSourceDeclarationProjectionAuthority(
	t *testing.T,
	pool *pgxpool.Pool,
	id int,
	workKind string,
	language string,
	rawResponse string,
) {
	t.Helper()
	commands := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO station_gap_openings VALUES (
			$1,$2,jsonb_build_object('language',$3::text)::text
		)`, []any{id, workKind, language}},
		{`INSERT INTO station_provider_discoveries VALUES ($1,$1)`, []any{id}},
		{`INSERT INTO station_provider_discovery_receipts VALUES ($1,'succeeded')`, []any{id}},
		{`INSERT INTO station_call_openings VALUES ($1,$1)`, []any{id}},
		{`INSERT INTO station_call_receipts VALUES (
			$1,'succeeded',jsonb_build_object('content',$2::text)::text,repeat('c',64)
		)`, []any{id, rawResponse}},
		{`INSERT INTO llm_call_evidence VALUES (
			$1,$1,encode(digest($2::text,'sha256'),'hex')
        )`, []any{id, rawResponse}},
	}
	for _, command := range commands {
		if _, err := pool.Exec(t.Context(), command.query, command.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func insertSourceDeclarationProjectionOutcome(
	t *testing.T,
	pool *pgxpool.Pool,
	id int,
	rawResponse string,
	declaration string,
	startByte int,
) error {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
        INSERT INTO station_gap_outcomes (
            id,opening_id,status,response,response_sha256,error,projection_kind,
            call_receipt_sha256,source_response_sha256,source_start_byte,source_end_byte
		) VALUES (
			$1,$1,'resolved',$2,encode(digest($2::text,'sha256'),'hex'),NULL,
			'source_declaration',repeat('c',64),encode(digest($3::text,'sha256'),'hex'),$4,$5
        )
    `, id, declaration, rawResponse, startByte, startByte+len(declaration))
	return err
}
