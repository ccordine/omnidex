package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTargetTreeLeafRequiresOneContentJob(t *testing.T) {
	program := directCodingProgram{StructureTransitions: []assemblyline.TargetTreeTransition{{Kind: assemblyline.TargetTreeCreate, Path: "src/Counter.tsx"}}}
	_, err := directCodingAssemblyFromProgram(program)
	if err == nil || !strings.Contains(err.Error(), "has no code-owned file-content job") {
		t.Fatalf("error=%v", err)
	}
}
