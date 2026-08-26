package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDeploymentDispositionUsesOneOpaqueSemanticCall(t *testing.T) {
	t.Parallel()
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 4, CorrectionModel: "forbidden",
		Execute: testPortableExecutor(func(_, model, prompt string, schema map[string]any) (string, error) {
			calls++
			if model != "disposition-model" || !strings.Contains(prompt, "DEPLOYMENT_CANDIDATE_2") {
				t.Fatalf("model=%q prompt=%q schema=%+v", model, prompt, schema)
			}
			for _, forbidden := range []string{"docker compose", "workspace", "command", "credential", "port"} {
				if strings.Contains(strings.ToLower(prompt), forbidden) {
					t.Fatalf("deployment prompt exposed %q: %s", forbidden, prompt)
				}
			}
			return fmt.Sprintf(`{"schema":%q,"candidate_id":%q}`,
				assemblyline.ApplicationServiceDeploymentIntentSchemaV1,
				assemblyline.ApplicationServiceDeploymentCurrentHostCandidate,
			), nil
		}),
	}
	resolution, err := resolveDirectCodingServiceDeploymentDisposition(
		runtime, "disposition-model", "Keep the completed appointment service running here.", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || resolution.Disposition != assemblyline.ApplicationServiceDeploymentPersistCurrentHost ||
		len(resolution.IntentJobID) != 64 || len(resolution.ResponseSHA256) != 64 {
		t.Fatalf("calls=%d resolution=%+v", calls, resolution)
	}
}

func TestDeploymentDispositionFailsClosedWithoutCorrection(t *testing.T) {
	t.Parallel()
	for name, candidate := range map[string]string{
		"malformed": `{"schema":"bad","candidate_id":"DEPLOYMENT_CANDIDATE_2"}`,
		"external": fmt.Sprintf(`{"schema":%q,"candidate_id":%q}`,
			assemblyline.ApplicationServiceDeploymentIntentSchemaV1,
			assemblyline.ApplicationServiceDeploymentOtherTargetCandidate,
		),
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 9, CorrectionModel: "forbidden",
				Execute: testPortableExecutor(func(_, _, _ string, _ map[string]any) (string, error) {
					calls++
					return candidate, nil
				}),
			}
			if _, err := resolveDirectCodingServiceDeploymentDisposition(
				runtime, "model", "Publish the weather endpoint to a remote platform.", nil,
			); err == nil || calls != 1 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
		})
	}
}
