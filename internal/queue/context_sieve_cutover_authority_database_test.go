package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresContextSieveCutoverOwnsOnlyExactNewStationsAndIndexes(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "127"),
	); err != nil {
		t.Fatal(err)
	}

	var terms, relevance, minification, wrongStation, crossKind, nested bool
	var historicalRequirements, historicalFileContent, historicalRepair bool
	var historicalReviewWorkload, historicalReviewStation bool
	var historicalConversation, historicalMemory, historicalContinuity bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'context_search_terms','context_search_terms','{}'::jsonb
			),
			station_owns_portable_work(
				'context_relevance','context_relevance','{}'::jsonb
			),
			station_owns_portable_work(
				'context_minification','context_minification','{}'::jsonb
			),
			station_owns_portable_work(
				'conversation_response','context_search_terms','{}'::jsonb
			),
			station_owns_portable_work(
				'context_search_terms','context_relevance','{}'::jsonb
			),
			station_owns_portable_work(
				'context_minification','response_correction',
				'{"original":{"kind":"context_minification","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_requirements','application_requirements','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_workload','application_file_content','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_workload','application_job_specification_repair','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_workload','application_job_specification_review','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_workload_review','application_job_specification_review','{}'::jsonb
			),
			station_owns_portable_work(
				'conversation_context_selection','conversation_context_selection','{}'::jsonb
			),
			station_owns_portable_work(
				'memory_context_selection','memory_context_selection','{}'::jsonb
			),
			station_owns_portable_work(
				'roleplay_narrative_continuity','roleplay_narrative_continuity','{}'::jsonb
			)
	`).Scan(
		&terms, &relevance, &minification, &wrongStation, &crossKind, &nested,
		&historicalRequirements, &historicalFileContent, &historicalRepair,
		&historicalReviewWorkload, &historicalReviewStation,
		&historicalConversation, &historicalMemory, &historicalContinuity,
	); err != nil {
		t.Fatal(err)
	}
	if !terms || !relevance || !minification || wrongStation || crossKind || !nested {
		t.Fatalf(
			"new station authority terms/relevance/minification/wrong/cross/nested=%t/%t/%t/%t/%t/%t",
			terms, relevance, minification, wrongStation, crossKind, nested,
		)
	}
	if !historicalConversation || !historicalMemory || !historicalContinuity {
		t.Fatalf(
			"immutable historical station authority conversation/memory/continuity=%t/%t/%t",
			historicalConversation, historicalMemory, historicalContinuity,
		)
	}
	if !historicalRequirements || !historicalFileContent || !historicalRepair ||
		!historicalReviewWorkload || !historicalReviewStation {
		t.Fatalf(
			"immutable legacy authority requirements/file/repair/review-workload/review-station=%t/%t/%t/%t/%t",
			historicalRequirements, historicalFileContent, historicalRepair,
			historicalReviewWorkload, historicalReviewStation,
		)
	}
	assertContextSieveNewStationsOpen(t, repository, pool)
	assertRetiredContextOpeningsRejected(t, pool)
	var sieveGuardSHA256 string
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
		FROM pg_proc
		WHERE oid='enforce_context_sieve_station_opening_insert()'::regprocedure
	`).Scan(&sieveGuardSHA256); err != nil {
		t.Fatal(err)
	}
	if sieveGuardSHA256 != "d6a479f722926498992a63d043383b8c313a643fa8b310d3303571cb558a04e0" {
		t.Fatalf("context sieve opening guard hash=%q", sieveGuardSHA256)
	}

	rows, err := pool.Query(t.Context(), `
		SELECT index_relation.relname, indexed_relation.relname,
		       access_method.amname,
		       pg_get_expr(index_authority.indexprs,index_authority.indrelid),
		       index_authority.indisvalid AND index_authority.indisready
		FROM pg_index AS index_authority
		JOIN pg_class AS index_relation
		  ON index_relation.oid=index_authority.indexrelid
		JOIN pg_class AS indexed_relation
		  ON indexed_relation.oid=index_authority.indrelid
		JOIN pg_am AS access_method ON access_method.oid=index_relation.relam
		WHERE index_relation.relname IN (
			'idx_ai_channel_messages_content_fts',
			'idx_roleplay_canon_events_content_fts',
			'idx_roleplay_character_memories_content_fts'
		)
		ORDER BY index_relation.relname
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	indexed := map[string]string{}
	for rows.Next() {
		var name, relation, method, expression string
		var valid bool
		if err := rows.Scan(&name, &relation, &method, &expression, &valid); err != nil {
			t.Fatal(err)
		}
		if method != "gin" || expression != "to_tsvector('simple'::regconfig, content)" || !valid {
			t.Fatalf("index %s method/expression/valid=%q/%q/%t", name, method, expression, valid)
		}
		indexed[name] = relation
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"idx_ai_channel_messages_content_fts":         "ai_channel_messages",
		"idx_roleplay_canon_events_content_fts":       "roleplay_canon_events",
		"idx_roleplay_character_memories_content_fts": "roleplay_character_memories",
	}
	if len(indexed) != len(want) {
		t.Fatalf("context FTS index count=%d want %d", len(indexed), len(want))
	}
	for name, relation := range want {
		if indexed[name] != relation {
			t.Fatalf("context FTS index %s relation=%q want %q", name, indexed[name], relation)
		}
	}
}

func assertContextSieveNewStationsOpen(
	t *testing.T,
	repository *Repository,
	pool *pgxpool.Pool,
) {
	t.Helper()
	jobAuthority, err := repository.EnqueueJob(
		t.Context(), "context-sieve-new-openings", model.PipelineCoding, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "context-sieve-opening-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != jobAuthority.ID {
		t.Fatalf("claim=%+v want job %d", claim, jobAuthority.ID)
	}
	termsJob := historicalContextSievePortableJob(t, "context_search_terms", 1)
	relevanceJob := historicalContextSievePortableJob(t, "context_relevance", 2)
	minificationJob := historicalContextSievePortableJob(t, "context_minification", 3)
	retainedCorrection := historicalContextSieveCorrectionJob(t, termsJob)
	applicationJob := historicalContextSievePortableJob(
		t, "application_job_specification", 4,
	)
	applicationCorrection := historicalContextSieveCorrectionJob(t, applicationJob)
	jobs := []struct {
		station station.ID
		job     assemblyline.PortableJob
	}{
		{station: station.ContextSearchTerms, job: termsJob},
		{station: station.ContextRelevance, job: relevanceJob},
		{station: station.ContextMinification, job: minificationJob},
		{station: station.ContextSearchTerms, job: retainedCorrection},
		{station: station.CodingWorkload, job: applicationCorrection},
	}
	for _, fixture := range jobs {
		opening := historicalContextSieveOpening(
			t, claim, fixture.job, fixture.station,
		)
		insertContextSieveMigrationOpening(t, pool, &opening)
	}
}

func historicalContextSievePortableJob(
	t *testing.T,
	kind string,
	marker int,
) assemblyline.PortableJob {
	t.Helper()
	payload := mustCanonical(t, struct {
		HistoricalMarker int `json:"historical_marker"`
	}{HistoricalMarker: marker})
	job := assemblyline.PortableJob{
		Schema:  "omnidex.portable-job.v1",
		Kind:    assemblyline.WorkKind(kind),
		Payload: payload,
	}
	job.ID = historicalPortableID(job.Schema, string(job.Kind), job.Payload)
	return job
}

func historicalContextSieveCorrectionJob(
	t *testing.T,
	original assemblyline.PortableJob,
) assemblyline.PortableJob {
	t.Helper()
	payload := mustCanonical(t, struct {
		Original          assemblyline.PortableJob `json:"original"`
		ValidationFailure string                   `json:"validation_failure"`
		RetainedCandidate string                   `json:"retained_candidate"`
	}{
		Original:          original,
		ValidationFailure: "historical candidate requires one correction",
		RetainedCandidate: "historical retained candidate",
	})
	job := assemblyline.PortableJob{
		Schema:  "omnidex.portable-job.v1",
		Kind:    assemblyline.WorkResponseCorrection,
		Payload: payload,
	}
	job.ID = historicalPortableID(job.Schema, string(job.Kind), job.Payload)
	return job
}

func assertRetiredContextOpeningsRejected(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	retired := []string{
		"application_requirements",
		"application_file_content",
		"application_job_specification_repair",
		"application_job_specification_review",
		"conversation_context_selection",
		"memory_context_selection",
		"roleplay_narrative_continuity",
	}
	for _, workKind := range retired {
		_, err := pool.Exec(t.Context(), `
			INSERT INTO station_gap_openings (work_kind) VALUES ($1)
		`, workKind)
		if err == nil || !strings.Contains(
			err.Error(), "retired station work kind "+workKind+" cannot create a new opening",
		) {
			t.Fatalf("retired work kind %s insert error=%v", workKind, err)
		}
	}
	for _, originalKind := range retired {
		payload := `{"original":{"kind":"` + originalKind + `"},` +
			`"retained_candidate":"{}"}`
		_, err := pool.Exec(t.Context(), `
			INSERT INTO station_gap_openings (work_kind,portable_payload)
			VALUES ('response_correction',$1)
		`, payload)
		if err == nil || !strings.Contains(
			err.Error(), "retired station work kind "+originalKind+
				" cannot create a correction opening",
		) {
			t.Fatalf("retired correction kind %s insert error=%v", originalKind, err)
		}
	}
	nestedPayload := `{"original":{"kind":"response_correction","payload":{` +
		`"original":{"kind":"conversation_context_selection","payload":{}}}}}`
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO station_gap_openings (work_kind,portable_payload)
		VALUES ('response_correction',$1)
	`, nestedPayload); err == nil || !strings.Contains(
		err.Error(), "nested response correction cannot create a new station opening",
	) {
		t.Fatalf("nested response correction insert error=%v", err)
	}

	for _, fixture := range []struct {
		name    string
		payload string
	}{
		{name: "missing", payload: `{"original":{"kind":"context_search_terms"}}`},
		{name: "blank", payload: `{"original":{"kind":"context_search_terms"},` +
			`"retained_candidate":""}`},
		{name: "whitespace", payload: `{"original":{"kind":"context_search_terms"},` +
			`"retained_candidate":"  "}`},
		{name: "non-string", payload: `{"original":{"kind":"context_search_terms"},` +
			`"retained_candidate":{}}`},
	} {
		t.Run("generic-correction-"+fixture.name, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), `
				INSERT INTO station_gap_openings (work_kind,portable_payload)
				VALUES ('response_correction',$1)
			`, fixture.payload)
			if err == nil || !strings.Contains(
				err.Error(),
				"response correction for context_search_terms requires one non-blank retained_candidate",
			) {
				t.Fatalf("generic correction payload %s insert error=%v", fixture.payload, err)
			}
		})
	}
}
