package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionSourceHasNoRepairRejectionRetryBoundary(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	forbidden := []string{
		"PriorRejection",
		"TypeScriptRepairGuidanceRejection",
		"FragmentRepairGuidanceRejection",
		"runDirectCodingTypeScriptRepairGuidanceAfterRejection",
		"coding_fragment_repair_guidance_rejected",
		"REJECTED_INSTRUCTION_JSON",
		"EXACT_INSTRUCTION_FAILURE",
		"maxDirectCodingLanguageExecutorAttempts",
		"acceptLanguageRepairGuidance",
		"acceptLanguageRepairSource",
		"CompilerConverge",
		"ConvergeExactTypeScriptStation",
		"ExactTypeScriptConvergence",
		"exactTypeScriptReplayCompiler",
		"exactConvergenceGap",
		"guided-typescript-convergence",
		"compiler-converge",
		"guidance-model",
	}
	for _, relative := range []string{"cmd", "internal"} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, token := range forbidden {
				if strings.Contains(source, token) {
					t.Errorf("production source %s retains repair-rejection retry boundary %q", path, token)
				}
			}
		})
	}
	for _, relative := range []string{
		"internal/worker/exact_station_convergence_loop.go",
		"internal/worker/exact_station_convergence_replay.go",
		"internal/worker/exact_station_convergence_types.go",
		"internal/worker/exact_station_convergence_progress.go",
		"internal/worker/exact_station_convergence_compiler.go",
	} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("retired exact-station convergence source remains: %s", path)
		}
	}
}
