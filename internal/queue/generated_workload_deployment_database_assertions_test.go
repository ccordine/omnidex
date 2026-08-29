package queue

import (
	"strings"
	"testing"
)

func assertGeneratedDeploymentStoredTextIsSecretFree(
	t *testing.T,
	fixture generatedDeploymentDatabaseFixture,
) {
	t.Helper()
	var stored string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT command_json || COALESCE(receipt_json,'') || COALESCE(evidence.payload_json::TEXT,'')
		FROM generated_workload_deployments AS deployment
		LEFT JOIN evidence ON evidence.id=deployment.evidence_id WHERE deployment.job_id=$1
	`, fixture.jobID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		fixture.rawSecret, fixture.rawConfig, "secret_value", "environment_json", "stdout", "stderr",
	} {
		if strings.Contains(strings.ToLower(stored), strings.ToLower(forbidden)) {
			t.Fatalf("generated deployment journal persisted forbidden payload %q", forbidden)
		}
	}
}
