package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryCognitionShadowHasOneMandatoryOutputBlindHook(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_existing_repository_workflow.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	selectAt := strings.Index(source, "surface, err := selectExistingRepositoryChangeSurface(")
	hookAt := strings.Index(source, "session.runRepositoryCognitionShadow(decision, pack.AnalysisID)")
	contractAt := strings.Index(source, "contract, err := session.buildExistingRepositoryChangeContract(")
	if selectAt < 0 || hookAt < 0 || contractAt < 0 || !(selectAt < hookAt && hookAt < contractAt) {
		t.Fatalf("repository cognition hook order select=%d hook=%d contract=%d", selectAt, hookAt, contractAt)
	}
	if strings.Contains(source, "runRepositoryCognitionShadow(redacted") {
		t.Fatal("repository cognition hook receives broad conversation authority")
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	invocations := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		contents, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		text := string(contents)
		invocations += strings.Count(text, ".runRepositoryCognitionShadow(")
		for _, forbidden := range []string{
			"StartCognitionEpisode(", "SealCognitionEpisode(",
			"CognitionTerminalCommand", "ObligationGraphSnapshot",
			"internal/labyrinth", "internal/cognitiongauntlet",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("worker production source %s contains forbidden cognition authority %q", file, forbidden)
			}
		}
	}
	if invocations != 1 {
		t.Fatalf("repository cognition shadow invocation count=%d, want exactly one", invocations)
	}
	shadow, err := os.ReadFile("v3_repository_cognition_shadow.go")
	if err != nil {
		t.Fatal(err)
	}
	start, err := os.ReadFile("v3_repository_cognition_start.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shadow), "facts := cognitionstate.NewNoFactAcceptanceAuthority()") ||
		!strings.Contains(string(shadow), "cognitionstore.New(") ||
		!strings.Contains(string(start), "store.StartEpisode(") {
		t.Fatal("repository cognition does not bind one explicit no-facts store/start authority")
	}
}
