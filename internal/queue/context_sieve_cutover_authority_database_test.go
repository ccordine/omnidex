package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
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
	assertContextSieveNewStationsOpen(t, repository)
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

func assertContextSieveNewStationsOpen(t *testing.T, repository *Repository) {
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
	candidate, err := assemblyline.NewContextCandidateAuthority(
		"conversation_user", "CTX_1", "The prior action adjusted the antenna.",
	)
	if err != nil {
		t.Fatal(err)
	}
	termsJob := mustContextSearchTermsJob(t)
	relevanceJob := mustContextRelevanceJob(t, candidate)
	minificationJob := mustContextMinificationJob(t, candidate)
	retainedCorrection, err := assemblyline.NewRetainedResponseCorrectionJob(
		termsJob,
		"one generated search term is not sufficiently specific",
		`{"schema":"omnidex.context-search-terms.v1","terms":["prior action"]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	jobs := []struct {
		station station.ID
		job     assemblyline.PortableJob
	}{
		{station: station.ContextSearchTerms, job: termsJob},
		{station: station.ContextRelevance, job: relevanceJob},
		{station: station.ContextMinification, job: minificationJob},
		{station: station.ContextSearchTerms, job: retainedCorrection},
		{station: station.CodingWorkload, job: mustApplicationJobSpecificationCorrection(t)},
		{station: station.CodingWorkloadReview, job: mustAcceptanceGroundingCorrection(t)},
	}
	for _, fixture := range jobs {
		if _, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
			Authority: claim.Authority,
			Job:       fixture.job, Station: fixture.station,
			ContextTokens: 32768, MaxOutputTokens: 32768,
			OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
		}); err != nil {
			t.Fatalf("open new context station %s: %v", fixture.station, err)
		}
	}
}

func mustApplicationJobSpecificationCorrection(t *testing.T) assemblyline.PortableJob {
	t.Helper()
	requirement := assemblyline.Requirement{
		ID: "requirement_001", SourceQuote: "filter inventory",
	}
	original, err := assemblyline.NewApplicationJobSpecificationJob(
		assemblyline.ApplicationJobSpecificationInput{
			Surface:              assemblyline.ApplicationSurfaceBrowser,
			ProductQuote:         "inventory console",
			AcceptedRequirements: []assemblyline.Requirement{requirement},
			FocusedRequirement:   requirement,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := assemblyline.NewResponseCorrectionJobForField(
		original, "the objective leaf is missing", "objective",
	)
	if err != nil {
		t.Fatal(err)
	}
	return correction
}

func mustAcceptanceGroundingCorrection(t *testing.T) assemblyline.PortableJob {
	t.Helper()
	const source = `function VerifyNotifications(): void {
  expect(screen.getByRole("checkbox", { name: "Email notices" })).toBeVisible();
}`
	input, err := assemblyline.NewApplicationAcceptanceGroundingReviewInput(
		assemblyline.ApplicationTaskContext{
			WorkloadSHA256: strings.Repeat("a", 64),
			Task: assemblyline.ApplicationTaskContextTask{
				TaskID: "task_004",
				AcceptanceCriteria: []string{
					"A user can find the email-notice control by its accessible name.",
				},
			},
		},
		source,
		true,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	original, err := assemblyline.NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := assemblyline.NewResponseCorrectionJobForField(
		original, "the grounding leaf is absent", "site_001__criterion_001",
	)
	if err != nil {
		t.Fatal(err)
	}
	return correction
}

func mustContextSearchTermsJob(t *testing.T) assemblyline.PortableJob {
	t.Helper()
	job, err := assemblyline.NewContextSearchTermsJob(
		assemblyline.ContextSearchTermsInput{ExactInstruction: "Do it again."},
	)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func mustContextRelevanceJob(
	t *testing.T,
	candidate assemblyline.ContextCandidateAuthority,
) assemblyline.PortableJob {
	t.Helper()
	job, err := assemblyline.NewContextRelevanceJob(assemblyline.ContextRelevanceInput{
		ExactInstruction: "Do it again.", RetrievalConcepts: []string{"previous action"},
		CandidateAuthorities: []assemblyline.ContextCandidateAuthority{candidate},
		MaxSelections:        1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func mustContextMinificationJob(
	t *testing.T,
	candidate assemblyline.ContextCandidateAuthority,
) assemblyline.PortableJob {
	t.Helper()
	job, err := assemblyline.NewContextMinificationJob(assemblyline.ContextMinificationInput{
		ExactInstruction: "Do it again.", SelectedAuthorities: []assemblyline.ContextCandidateAuthority{candidate},
	})
	if err != nil {
		t.Fatal(err)
	}
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
