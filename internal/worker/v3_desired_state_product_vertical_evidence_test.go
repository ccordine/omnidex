package worker

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/evidence"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

func assertDesiredStateProductStationEvidence(
	t *testing.T,
	pool *pgxpool.Pool,
	jobID int64,
	calls []desiredStateProductModelCall,
	test desiredStateProductCase,
) {
	t.Helper()
	assertDesiredStateProductStationPersistence(t, pool, jobID, calls, test.wantKinds)
	assertDesiredStateProductModelAuthority(t, calls, test)
}

func assertDesiredStateProductStationPersistence(
	t *testing.T,
	pool *pgxpool.Pool,
	jobID int64,
	calls []desiredStateProductModelCall,
	wantKinds []assemblyline.WorkKind,
) {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT opening.work_kind,opening.prompt,outcome.response,
		       calls.model_input,calls.protocol
		FROM station_gap_openings AS opening
		JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
		JOIN station_call_openings AS calls ON calls.gap_opening_id=opening.id
		WHERE opening.job_id=$1
		ORDER BY opening.id
	`, jobID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	kinds := make([]assemblyline.WorkKind, 0, len(calls))
	index := 0
	for rows.Next() {
		var kind assemblyline.WorkKind
		var prompt, response, modelInput, protocol string
		if err := rows.Scan(&kind, &prompt, &response, &modelInput, &protocol); err != nil {
			t.Fatal(err)
		}
		if index >= len(calls) {
			t.Fatalf("durable station evidence contains unexpected call %s", kind)
		}
		call := calls[index]
		if call.Prompt != prompt || call.Response != response || string(call.Protocol) != protocol ||
			!strings.Contains(modelInput, prompt) {
			t.Fatalf("provider and durable exact envelope differ at call %d kind=%s", index, kind)
		}
		kinds = append(kinds, kind)
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if index != len(calls) || !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("exact station kinds=%v provider_calls=%d want=%v", kinds, len(calls), wantKinds)
	}

	var contextSelections, memorySelections, successors int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE station='conversation_context_selection'),
			COUNT(*) FILTER (WHERE station='memory_context_selection')
		FROM station_gap_openings WHERE job_id=$1
	`, jobID).Scan(&contextSelections, &memorySelections); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM jobs
		WHERE project_id=(SELECT project_id FROM jobs WHERE id=$1) AND id<>$1
	`, jobID).Scan(&successors); err != nil {
		t.Fatal(err)
	}
	if contextSelections != 0 || memorySelections != 0 || successors != 0 {
		t.Fatalf("continuity/successor calls=%d/%d/%d want 0/0/0", contextSelections, memorySelections, successors)
	}
}

func assertDesiredStateProductModelAuthority(
	t *testing.T,
	calls []desiredStateProductModelCall,
	test desiredStateProductCase,
) {
	t.Helper()
	assertDesiredStateProductNoModelMutationOps(t, calls)
	for index, call := range calls {
		if test.present && strings.Contains(call.Prompt, test.target) {
			t.Fatalf("model call %d saw code-derived target path %q", index, test.target)
		}
		if !test.present && strings.Contains(call.Prompt, test.target) {
			t.Fatalf(
				"model call %d saw deletion target path %q; exact free-form objective classification and zero model-visible target paths cannot both hold",
				index, test.target,
			)
		}
	}
	for _, call := range calls {
		if strings.Contains(call.Response, test.target) {
			t.Fatalf("model selected target path in response %q", call.Response)
		}
	}
}

func assertDesiredStateProductNoModelMutationOps(
	t *testing.T,
	calls []desiredStateProductModelCall,
) {
	t.Helper()
	shellMutation := regexp.MustCompile(`(?i)\b(?:rm|mv|sed)\s+(?:-|[^[:space:]])`)
	for index, call := range calls {
		lowerPrompt := strings.ToLower(call.Prompt)
		lowerResponse := strings.ToLower(call.Response)
		for _, operation := range []string{
			"create_file", "delete_file", "rename_file", "move_file", "write_file",
			"shell command",
		} {
			if strings.Contains(lowerPrompt, operation) || strings.Contains(lowerResponse, operation) {
				t.Fatalf("model call %d exposed or selected mutation operation %q", index, operation)
			}
		}
		if shellMutation.MatchString(call.Prompt) || shellMutation.MatchString(call.Response) {
			t.Fatalf("model call %d exposed or selected a shell mutation command", index)
		}
	}
}

func assertDesiredStateProductMutationEvidence(
	t *testing.T,
	pool *pgxpool.Pool,
	projectID, jobID int64,
	before, after repositoryfacts.Snapshot,
	test desiredStateProductCase,
) {
	t.Helper()
	var status, sourceSnapshotID, path string
	var attempts int
	var sourcePresent, expectedPresent bool
	var sourceSHA, expectedSHA *string
	if err := pool.QueryRow(t.Context(), `
		SELECT operation.status,operation.attempt_count,operation.source_snapshot_id,
		       file.path,file.source_present,file.expected_present,
		       file.source_sha256,file.expected_sha256
		FROM repository_mutation_operations AS operation
		JOIN repository_mutation_files AS file ON file.operation_id=operation.id
		WHERE operation.job_id=$1
	`, jobID).Scan(
		&status, &attempts, &sourceSnapshotID, &path, &sourcePresent, &expectedPresent,
		&sourceSHA, &expectedSHA,
	); err != nil {
		t.Fatal(err)
	}
	if status != "applied" || attempts != 1 || sourceSnapshotID != before.ID ||
		path != test.target || sourcePresent == test.present || expectedPresent != test.present {
		t.Fatalf(
			"journal status=%s attempts=%d snapshot=%s path=%s transition=%t->%t",
			status, attempts, sourceSnapshotID, path, sourcePresent, expectedPresent,
		)
	}
	if test.present {
		file := desiredStateProductSnapshotFile(t, after, test.target)
		if sourceSHA != nil || expectedSHA == nil || *expectedSHA != file.SHA256 {
			t.Fatalf("created file journal source=%v expected=%v actual=%s", sourceSHA, expectedSHA, file.SHA256)
		}
	} else {
		file := desiredStateProductSnapshotFile(t, before, test.target)
		if sourceSHA == nil || *sourceSHA != file.SHA256 || expectedSHA != nil {
			t.Fatalf("deleted file journal source=%v actual=%s expected=%v", sourceSHA, file.SHA256, expectedSHA)
		}
	}

	var snapshots, currentTarget, graphEvidence, indexEvidence int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM repository_snapshots
		WHERE project_id=$1 AND id IN ($2,$3)
	`, projectID, before.ID, after.ID).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM repository_files WHERE snapshot_id=$1 AND path=$2
	`, after.ID, test.target).Scan(&currentTarget); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE kind=$2),
			COUNT(*) FILTER (WHERE kind=$3)
		FROM evidence WHERE job_id=$1
	`, jobID, evidence.KindRepositoryDesiredGraph, evidence.KindRepositoryIndex).Scan(
		&graphEvidence, &indexEvidence,
	); err != nil {
		t.Fatal(err)
	}
	wantTarget := 0
	if test.present {
		wantTarget = 1
	}
	if snapshots != 2 || currentTarget != wantTarget || graphEvidence != 1 || indexEvidence != 2 {
		t.Fatalf(
			"reindex evidence snapshots=%d target=%d graph=%d index=%d",
			snapshots, currentTarget, graphEvidence, indexEvidence,
		)
	}

	var generatedDiffs, baselineProofs, baselineAcceptances, stagedProofs, authoritativeProofs int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE kind=$2 AND source_type='repository'),
			COUNT(*) FILTER (WHERE kind=$3 AND payload_json->'metadata'->>'repository_verification_scope'='baseline'
			  AND NOT COALESCE((payload_json->'metadata'->>'repository_verification_baseline_accepted')::boolean,false)),
			COUNT(*) FILTER (WHERE kind=$3 AND COALESCE((payload_json->'metadata'->>'repository_verification_baseline_accepted')::boolean,false)),
			COUNT(*) FILTER (WHERE kind=$3 AND payload_json->'metadata'->>'repository_verification_scope'='staged'
			  AND NOT COALESCE((payload_json->'metadata'->>'repository_verification_plan_accepted')::boolean,false)),
			COUNT(*) FILTER (WHERE kind=$3 AND payload_json->'metadata'->>'repository_verification_scope'='authoritative'
			  AND NOT COALESCE((payload_json->'metadata'->>'repository_verification_plan_accepted')::boolean,false))
		FROM evidence WHERE job_id=$1
	`, jobID, evidence.KindGeneratedDiff, evidence.KindTestResult).Scan(
		&generatedDiffs, &baselineProofs, &baselineAcceptances, &stagedProofs, &authoritativeProofs,
	); err != nil {
		t.Fatal(err)
	}
	if generatedDiffs != 1 || baselineProofs != 2 || baselineAcceptances != 1 ||
		stagedProofs != 2 || authoritativeProofs != 2 {
		t.Fatalf(
			"verification evidence diff=%d baseline=%d/%d staged=%d authoritative=%d",
			generatedDiffs, baselineProofs, baselineAcceptances, stagedProofs, authoritativeProofs,
		)
	}
}

func desiredStateProductSnapshotFile(
	t *testing.T,
	snapshot repositoryfacts.Snapshot,
	path string,
) repositoryfacts.File {
	t.Helper()
	for _, file := range snapshot.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("snapshot %s omitted %q", snapshot.ID, path)
	return repositoryfacts.File{}
}
