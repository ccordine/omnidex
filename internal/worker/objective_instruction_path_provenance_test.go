package worker

import (
	"context"
	"os"
	"path/filepath"
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
