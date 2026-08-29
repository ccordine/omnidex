package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDeploymentLifecycleManifestDerivesExactStatelessAndStatefulSlots(t *testing.T) {
	t.Parallel()
	descriptor := *genericPHPDeploymentDescriptor()
	environment := map[string]string{
		"HOST_BIND_ADDRESS": "127.0.0.1", "HOST_HTTP_PORT": "0",
		"SERVICE_STATE_DB_PASSWORD": "secret",
	}
	for _, testCase := range []struct {
		name     string
		hasState bool
		want     []string
	}{
		{name: "stateless", want: []string{
			"build", "initial_start", "initial_observe", "restart", "restart_start", "final_observe",
		}},
		{name: "stateful", hasState: true, want: []string{
			"build", "initial_start", "migrate", "initial_observe", "state_write",
			"restart", "restart_start", "final_observe", "state_read",
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			manifest, err := directCodingDeploymentLifecycleManifest(
				"omnidex-job-7-g1", descriptor, environment, strings.Repeat("a", 64), testCase.hasState,
			)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(manifest.Commands))
			for index, execution := range manifest.Commands {
				got[index] = execution.Slot.Name
				if len(execution.CommandSHA256) != 64 || execution.WorkspaceSHA256 != strings.Repeat("a", 64) ||
					execution.Slot == queue.GeneratedDeploymentSlotConfig || execution.Slot == queue.GeneratedDeploymentSlotRollback {
					t.Fatalf("execution %d=%+v", index, execution)
				}
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("slots=%v want=%v", got, testCase.want)
			}
		})
	}
}

func TestDeploymentLifecycleManifestRejectsStateWithoutMigrationAuthority(t *testing.T) {
	descriptor := *genericPHPDeploymentDescriptor()
	descriptor.MigrationScript = ""
	_, err := directCodingDeploymentLifecycleManifest(
		"omnidex-job-7-g1", descriptor,
		map[string]string{"HOST_BIND_ADDRESS": "127.0.0.1", "HOST_HTTP_PORT": "0"},
		strings.Repeat("a", 64), true,
	)
	if err == nil || !strings.Contains(err.Error(), "migration operation") {
		t.Fatalf("missing state migration error=%v", err)
	}
}
