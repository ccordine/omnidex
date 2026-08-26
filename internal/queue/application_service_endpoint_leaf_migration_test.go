package queue

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const applicationServiceEndpointLeafMigration = "137_application_service_endpoint_leaf_stations.sql"

func TestApplicationServiceEndpointLeafMigrationRegistersLeavesAndRetiresBundledWork(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + applicationServiceEndpointLeafMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"cannot retire bundled service endpoint contract while an active opening is unresolved",
		"WHEN 'application_service_endpoint_exposure' THEN station='coding_service_endpoint_exposure'",
		"WHEN 'application_service_endpoint_method' THEN station='coding_service_endpoint_method'",
		"WHEN 'application_service_endpoint_route_template' THEN station='coding_service_endpoint_route_template'",
		"WHEN 'application_service_endpoint_request_media' THEN station='coding_service_endpoint_request_media'",
		"WHEN 'application_service_endpoint_response_media' THEN station='coding_service_endpoint_response_media'",
		"WHEN 'application_service_endpoint_success_status' THEN station='coding_service_endpoint_success_status'",
		"WHEN 'application_service_endpoint_contract' THEN station='coding_service_endpoint_contract'",
		"'application_service_endpoint_contract', 'conversation_context_selection'",
		"efbe7a813ef0a0ad9df888bdca40f571fff58e5db6d261c4dcf9d0ee7229f5e1",
		"33bbce2c9ec84a87fa185494b40f8038fb9531592c6af64ff88fd927cb0922f1",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("service endpoint leaf migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("service endpoint leaf migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationServiceEndpointLeafMigrationOwnsEachLeafAndRejectsNewBundledWork(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "137")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationServiceEndpointLeafMigration, 1)

	owners := []struct {
		station string
		kind    string
	}{
		{"coding_service_endpoint_exposure", "application_service_endpoint_exposure"},
		{"coding_service_endpoint_method", "application_service_endpoint_method"},
		{"coding_service_endpoint_route_template", "application_service_endpoint_route_template"},
		{"coding_service_endpoint_request_media", "application_service_endpoint_request_media"},
		{"coding_service_endpoint_response_media", "application_service_endpoint_response_media"},
		{"coding_service_endpoint_success_status", "application_service_endpoint_success_status"},
	}
	for _, owner := range owners {
		var direct, wrong, correction bool
		if err := pool.QueryRow(t.Context(), `
			SELECT
				station_owns_portable_work($1,$2,'{}'::jsonb),
				station_owns_portable_work('not_the_owner',$2,'{}'::jsonb),
				station_owns_portable_work(
					$1,'response_correction',
					jsonb_build_object('original',jsonb_build_object('kind',$2,'payload','{}'::jsonb))
				)
		`, owner.station, owner.kind).Scan(&direct, &wrong, &correction); err != nil {
			t.Fatal(err)
		}
		if !direct || wrong || !correction {
			t.Fatalf("leaf %s direct/wrong/correction=%t/%t/%t", owner.kind, direct, wrong, correction)
		}
	}

	for _, fixture := range []struct {
		name    string
		kind    string
		payload string
		message string
	}{
		{
			name: "direct", kind: "application_service_endpoint_contract", payload: "{}",
			message: "retired station work kind application_service_endpoint_contract cannot create a new opening",
		},
		{
			name: "correction", kind: "response_correction",
			payload: `{"original":{"kind":"application_service_endpoint_contract","payload":{}}}`,
			message: "retired station work kind application_service_endpoint_contract cannot create a correction opening",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), `
				INSERT INTO station_gap_openings (work_kind,portable_payload)
				VALUES ($1,$2)
			`, fixture.kind, fixture.payload)
			if err == nil || !strings.Contains(err.Error(), fixture.message) {
				t.Fatalf("retired opening error=%v want %q", err, fixture.message)
			}
		})
	}
}

func TestApplicationServiceEndpointLeafMigrationPreservesCompletedBundledHistory(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "136")); err != nil {
		t.Fatal(err)
	}
	claim := contextSieveMigrationClaim(t, repository, "completed-bundled-service-endpoint")
	job := historicalBundledServiceEndpointJob(t)
	opening := historicalContextSieveOpening(
		t, claim, job, station.ID("coding_service_endpoint_contract"),
	)
	insertContextSieveMigrationOpening(t, pool, &opening)
	persistStationDiscoveryFailure(t, repository, claim.Authority, opening)
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: opening.ID, GapID: opening.GapID,
		Status: StationGapFailed, Error: "historical bundled endpoint failed before leaf cutover",
	}); err != nil {
		t.Fatal(err)
	}

	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "137")); err != nil {
		t.Fatalf("completed bundled endpoint history blocked leaf cutover: %v", err)
	}
	var retained, historicalOwner bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM station_gap_openings AS opening
			JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
			WHERE opening.id=$1
		), station_owns_portable_work(
			'coding_service_endpoint_contract','application_service_endpoint_contract','{}'::jsonb
		)
	`, opening.ID).Scan(&retained, &historicalOwner); err != nil {
		t.Fatal(err)
	}
	if !retained || !historicalOwner {
		t.Fatalf("completed bundled history retained/owned=%t/%t", retained, historicalOwner)
	}
}

func TestApplicationServiceEndpointLeafMigrationRejectsActiveBundledHistory(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "136")); err != nil {
		t.Fatal(err)
	}
	claim := contextSieveMigrationClaim(t, repository, "active-bundled-service-endpoint")
	opening := historicalContextSieveOpening(
		t, claim, historicalBundledServiceEndpointJob(t),
		station.ID("coding_service_endpoint_contract"),
	)
	insertContextSieveMigrationOpening(t, pool, &opening)

	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "137"))
	if err == nil || !strings.Contains(
		err.Error(), "cannot retire bundled service endpoint contract while an active opening is unresolved",
	) {
		t.Fatalf("active bundled endpoint migration error=%v", err)
	}
}

func historicalBundledServiceEndpointJob(t *testing.T) assemblyline.PortableJob {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"product_context":     "historical service",
		"requirement_quote":   "Expose a historical endpoint.",
		"objective":           "Expose one endpoint.",
		"required_behaviors":  []string{"Return one response."},
		"acceptance_criteria": []string{"The response is returned."},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := assemblyline.PortableJob{
		Schema:  assemblyline.PortableJobSchemaV1,
		Kind:    assemblyline.WorkKind("application_service_endpoint_contract"),
		Payload: payload,
	}
	job.ID = historicalPortableID(job.Schema, string(job.Kind), job.Payload)
	return job
}
