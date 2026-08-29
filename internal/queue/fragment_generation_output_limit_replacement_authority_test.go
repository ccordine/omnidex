package queue

import "testing"

func TestPostgresFragmentGenerationOutputLimitReplacementAuthority(t *testing.T) {
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(t.Context(), loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	var transportHash, receiptHash, lineageAuthorityHash string
	var stationLanguage, stationVolatility, receiptLanguage, receiptVolatility string
	var lineageLanguage, lineageVolatility string
	var stationStrict, receiptStrict, lineageStrict, lineageExact bool
	var receiptTriggerType int16
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
		FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass
		  AND conname='station_gap_openings_current_raw_transport'
		  AND convalidated
	`).Scan(&transportHash); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
		       language.lanname,procedure.provolatile::text,procedure.proisstrict
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure('require_station_call_receipt_before_gap_outcome()')
	`).Scan(&receiptHash, &receiptLanguage, &receiptVolatility, &receiptStrict); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex'),
		       language.lanname,procedure.provolatile::text,procedure.proisstrict,
		       fragment_generation_replacement_authority_is_exact()
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure(
			'fragment_generation_replacement_authority_is_exact()'
		)
	`).Scan(
		&lineageAuthorityHash, &lineageLanguage, &lineageVolatility,
		&lineageStrict, &lineageExact,
	); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT language.lanname,procedure.provolatile::text,procedure.proisstrict
		FROM pg_proc AS procedure
		JOIN pg_language AS language ON language.oid=procedure.prolang
		WHERE procedure.oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)')
	`).Scan(&stationLanguage, &stationVolatility, &stationStrict); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT tgtype
		FROM pg_trigger
		WHERE tgrelid='station_gap_outcomes'::regclass
		  AND tgname='station_gap_outcomes_require_call_receipt'
		  AND tgfoid=to_regprocedure('require_station_call_receipt_before_gap_outcome()')
		  AND tgenabled='O' AND NOT tgisinternal
	`).Scan(&receiptTriggerType); err != nil {
		t.Fatal(err)
	}
	if transportHash != "0295101bc9f22439463b3054efb15a715fcd1ee02fcfc3df8a69b903f595814a" ||
		receiptHash != "9ffb069bb0a14804df717b9cd918e167c3bfc88eede8f3cd744b39c1715ff303" ||
		stationLanguage != "sql" || stationVolatility != "i" || !stationStrict ||
		receiptLanguage != "plpgsql" || receiptVolatility != "v" || receiptStrict ||
		receiptTriggerType != 7 ||
		lineageAuthorityHash != "43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae" ||
		lineageLanguage != "sql" || lineageVolatility != "s" ||
		lineageStrict || !lineageExact {
		t.Fatalf(
			"replacement transport/receipt authority hashes=%s/%s station=%s/%s/%t receipt=%s/%s/%t trigger=%d lineage=%s/%s/%s/%t/%t",
			transportHash, receiptHash, stationLanguage, stationVolatility,
			stationStrict, receiptLanguage, receiptVolatility, receiptStrict,
			receiptTriggerType, lineageAuthorityHash, lineageLanguage,
			lineageVolatility, lineageStrict, lineageExact,
		)
	}
	var owns bool
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
			'coding_fragment','fragment_generation_replacement','{}'::jsonb
		)
	`).Scan(&owns); err != nil {
		t.Fatal(err)
	}
	if !owns {
		t.Fatal("coding fragment station does not own output-limit replacement")
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION replacement_json_nonnegative_integer_is_exact(
			value JSON,
			maximum NUMERIC
		)
		RETURNS BOOLEAN AS $$ SELECT TRUE $$
		LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT fragment_generation_replacement_authority_is_exact()
	`).Scan(&owns); err != nil {
		t.Fatal(err)
	}
	if owns {
		t.Fatal("replacement lineage authority accepted a modified numeric type validator")
	}
}
