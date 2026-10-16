package worker

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestObjectiveInstructionPathProvenanceDoesNotRequireGitOrContentCapture(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o000); err != nil {
		t.Fatal(err)
	}

	provenance, err := objectiveInstructionPathProvenance(
		context.Background(), root, "repair main.go",
	)
	if err != nil {
		t.Fatal(err)
	}
	redacted, identities, err := assemblyline.RedactArtifactIdentities(
		"repair main.go", provenance,
	)
	if err != nil {
		t.Fatal(err)
	}
	if redacted != "repair ARTIFACT_1" || len(identities) != 1 || identities[0].Value != "main.go" {
		t.Fatalf("unexpected redaction %q with %#v", redacted, identities)
	}
}

func TestObjectiveInstructionPathProvenanceRedactsEveryExplicitAdapterRecognizedPath(t *testing.T) {
	root := t.TempDir()
	provenance, err := objectiveInstructionPathProvenance(
		context.Background(), root, "repair main.go and one/main.go",
	)
	if err != nil {
		t.Fatal(err)
	}
	redacted, identities, err := assemblyline.RedactArtifactIdentities(
		"repair main.go and one/main.go", provenance,
	)
	if err != nil {
		t.Fatal(err)
	}
	if redacted != "repair ARTIFACT_1 and ARTIFACT_2" || len(identities) != 2 ||
		identities[0].Value != "main.go" || identities[1].Value != "one/main.go" {
		t.Fatalf("unexpected direct-path redaction %q with %#v", redacted, identities)
	}
}

func TestObjectiveInstructionPathProvenanceDoesNotTreatInlineJSONAsAPath(t *testing.T) {
	root := t.TempDir()
	instruction := "Build a CLI that reads `{ \"tasks\": [{\"id\":\"build\",\"depends_on\":[\"lint\"]}] }` and " +
		"`{\"route\":\"src/main.go\"}`. Write main.go and `docs/My Guide.md`."
	provenance, err := objectiveInstructionPathProvenance(
		context.Background(), root, instruction,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"docs/My Guide.md", "main.go"}
	if got := provenance.Paths(); !slices.Equal(got, want) {
		t.Fatalf("artifact provenance = %#v, want %#v", got, want)
	}
	gauntlet := "Build a dependency-free Go CLI named relayplan in this empty directory. It reads a JSON file (or `-` for stdin) shaped as " +
		"`{ \"tasks\": [{\"id\":\"build\",\"duration_minutes\":5,\"depends_on\":[\"lint\"],\"done\":false}] }`. " +
		"Provide validate, ready, and order subcommands. Run gofmt, go test ./..., and go vet ./... before calling it complete."
	gauntletProvenance, err := objectiveInstructionPathProvenance(context.Background(), root, gauntlet)
	if err != nil {
		t.Fatal(err)
	}
	redacted, identities, err := assemblyline.RedactArtifactIdentities(gauntlet, gauntletProvenance)
	if err != nil {
		t.Fatal(err)
	}
	if len(gauntletProvenance.Paths()) != 0 || len(identities) != 1 || identities[0].Value != "./" ||
		strings.Contains(redacted, "ARTIFACT_1 for stdin") || !strings.Contains(redacted, "shaped as `{ \"tasks\"") {
		t.Fatalf("gauntlet redaction = %q with paths %#v and identities %#v", redacted, gauntletProvenance.Paths(), identities)
	}
	applicationContext, err := assemblyline.BootstrapApplicationContext(redacted)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationRequirementInventoryJob(
		assemblyline.ApplicationRequirementInventoryInput{UserRequest: redacted, Context: applicationContext},
	)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePortableSemanticPrompt(prompt, identities, gauntletProvenance); err != nil {
		t.Fatalf("gauntlet semantic prompt boundary: %v", err)
	}
}

func TestObjectiveInstructionPathProvenanceDoesNotTreatEscapedOutputAsAPath(t *testing.T) {
	root := t.TempDir()
	instruction := "Write exactly `completed ID\\n` on success and `usage: relayplan FILE\\n` on misuse. " +
		"Implement cmd/relayplan/main.go and `docs/CLI Guide.md`."
	provenance, err := objectiveInstructionPathProvenance(context.Background(), root, instruction)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd/relayplan/main.go", "docs/CLI Guide.md"}
	if got := provenance.Paths(); !slices.Equal(got, want) {
		t.Fatalf("artifact provenance = %#v, want %#v", got, want)
	}
}

func TestObjectiveInstructionPathProvenanceUsesWindowsClientSeparators(t *testing.T) {
	provenance, err := objectiveInstructionPathProvenance(
		context.Background(), `C:\work\source`, `Edit cmd\relayplan\main.go and "docs\CLI Guide.md".`,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd/relayplan/main.go", "docs/CLI Guide.md"}
	if got := provenance.Paths(); !slices.Equal(got, want) {
		t.Fatalf("artifact provenance = %#v, want %#v", got, want)
	}
}

func TestObjectiveRelativeArtifactPathsUseClientPathSyntax(t *testing.T) {
	for _, fixture := range []struct{ root, candidate, want string }{
		{`C:\work\source`, `C:\work\source\src\main.go`, "src/main.go"},
		{`\\server\share\reports`, `\\server\share\reports\data\values.json`, "data/values.json"},
		{`C:\work\source`, `src\main.go`, "src/main.go"},
		{`C:\`, `C:\main.go`, "main.go"},
		{"/work/source", "/work/source/src/main.go", "src/main.go"},
	} {
		got, err := objectiveRelativeArtifactPath(fixture.root, fixture.candidate)
		if err != nil || got != fixture.want {
			t.Errorf("relative path %q against %q = %q: %v, want %q", fixture.candidate, fixture.root, got, err, fixture.want)
		}
	}
	for _, candidate := range []string{`C:\elsewhere\main.go`, `D:\work\source\main.go`, `..\main.go`, `C:main.go`, `C:\work\source\..\main.go`} {
		if _, err := objectiveRelativeArtifactPath(`C:\work\source`, candidate); err == nil {
			t.Errorf("accepted outside or drive-relative artifact %q", candidate)
		}
	}
}
