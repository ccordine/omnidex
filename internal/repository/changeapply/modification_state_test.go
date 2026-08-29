package changeapply_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestAssembleExistingGoFileStatesReturnsOneCompleteExactPostimage(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	contract := fixture.contract(t, "First")
	file := fixture.file(t, "first.go")
	symbolID := fixture.symbol(t, "First").ID
	candidates := map[string]string{
		symbolID: "func First() int {\r\n\treturn 9\r\n}",
	}
	desired, err := changeapply.AssembleExistingGoFileStates(
		fixture.snapshot, fixture.analysis, contract, candidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 {
		t.Fatalf("assembled desired states=%+v", desired)
	}
	want := []byte("package changeapply\n\nfunc First() int {\n\treturn 9\n}\n")
	if desired[0].Path != file.Path || !desired[0].Present ||
		desired[0].Source.FileID != file.ID || desired[0].Source.SHA256 != file.SHA256 ||
		desired[0].Source.Size != file.Size || desired[0].Source.Mode != file.Mode ||
		desired[0].Mode != file.Mode || desired[0].PackageArtifactID == "" ||
		!bytes.Equal(desired[0].Content, want) {
		t.Fatalf("assembled desired state=%+v content=%q", desired, desired[0].Content)
	}
	if candidates[symbolID] != "func First() int {\r\n\treturn 9\r\n}" {
		t.Fatalf("assembler mutated caller-owned candidates: %#v", candidates)
	}
	assertFile(
		t, filepath.Join(fixture.root, "first.go"),
		"package changeapply\n\nfunc First() int { return 1 }\n", 0o600,
	)
}
