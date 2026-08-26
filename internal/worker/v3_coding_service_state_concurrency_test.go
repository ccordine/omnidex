package worker

import (
	"strings"
	"testing"
)

func TestPHPStateRuntimesRejectLostConcurrentWrites(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{
		"generic_php": phpServiceStateRuntimeSource(),
		"laravel":     laravelStateRuntimeSource(),
	} {
		t.Run(name, func(t *testing.T) {
			for _, required := range []string{
				"must be loaded before it is saved",
				"AND revision =", "lost optimistic revision authority",
				"self::$revisions[$identity] = $revision + 1",
			} {
				if !strings.Contains(source, required) {
					t.Fatalf("state runtime omits optimistic-concurrency authority %q", required)
				}
			}
			if strings.Contains(source, "ON CONFLICT (state_scope, state_key) DO UPDATE") {
				t.Fatal("state runtime retained an unconditional whole-state overwrite")
			}
		})
	}
	if !strings.Contains(phpServiceStateRuntimeSource(), "SELECT state_value, revision") ||
		!strings.Contains(laravelStateRuntimeSource(), "first(['state_value', 'revision'])") {
		t.Fatal("state runtimes do not load the persisted revision with the state value")
	}
}
