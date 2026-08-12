package cognitiongauntlet

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSemanticPlanRevisionUsesFrozenPostMaterializationPhase(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate semantic plan revision mapper")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(current), "semantic_replay_memory.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "record.Phase != 44") ||
		strings.Contains(source, "record.Phase != 43") {
		t.Fatal("semantic plan revision mapper drifted from frozen phase 44")
	}
}
