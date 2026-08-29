package golang

import (
	"context"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestAnalyzeIndexesExactTypeValueUsesAndBuildMembership(t *testing.T) {
	root := newGoAnalysisRepository(t)
	writeGoAnalysisFile(t, root, "go.mod", "module example.test/references\n\ngo 1.24\n")
	writeGoAnalysisFile(t, root, "declarations.go", `package references

type Record struct { Value int }
const DefaultValue = 7
`)
	writeGoAnalysisFile(t, root, "consumer.go", `package references

func Consume(record Record) int { return record.Value + DefaultValue }
`)
	runGoAnalysisGit(t, root, "add", ".")
	runGoAnalysisGit(t, root, "commit", "-m", "typed references")
	snapshot, err := repositoryfacts.BuildGitSnapshot(t.Context(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFileToSymbolEdge(analysis, snapshot, "uses_type", "consumer.go", "example.test/references.Record") {
		t.Fatalf("analysis omitted exact type-use edge: %#v", analysis.Edges)
	}
	if !hasFileToSymbolEdge(analysis, snapshot, "uses_value", "consumer.go", "example.test/references.DefaultValue") {
		t.Fatalf("analysis omitted exact value-use edge: %#v", analysis.Edges)
	}
	for _, source := range []string{"consumer.go", "declarations.go"} {
		if !hasPackageToFileEdge(analysis, snapshot, "builds_from", "example.test/references", source) {
			t.Fatalf("analysis omitted build membership for %q: %#v", source, analysis.Edges)
		}
	}
}

func TestAnalyzeDoesNotConfuseLocalShadowingWithPackageValueUse(t *testing.T) {
	root := newGoAnalysisRepository(t)
	writeGoAnalysisFile(t, root, "go.mod", "module example.test/shadow\n\ngo 1.24\n")
	writeGoAnalysisFile(t, root, "global.go", "package shadow\n\nvar value = 7\n")
	writeGoAnalysisFile(t, root, "local.go", "package shadow\n\nfunc Local(value int) int { return value }\n")
	runGoAnalysisGit(t, root, "add", ".")
	runGoAnalysisGit(t, root, "commit", "-m", "shadow")
	snapshot, err := repositoryfacts.BuildGitSnapshot(t.Context(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if hasFileToSymbolEdge(analysis, snapshot, "uses_value", "local.go", "example.test/shadow.value") {
		t.Fatalf("local shadow became a false package-value dependency: %#v", analysis.Edges)
	}
}

func TestAnalyzeIndexesBlankImportRegistrationToExactInitializationSource(t *testing.T) {
	root := newGoAnalysisRepository(t)
	writeGoAnalysisFile(t, root, "go.mod", "module example.test/registration\n\ngo 1.24\n")
	writeGoAnalysisFile(t, root, "registry.go", `package registration

import _ "example.test/registration/plugin"
`)
	writeGoAnalysisFile(t, root, "plugin/retained.go", "package plugin\n\nfunc Retained() {}\n")
	writeGoAnalysisFile(t, root, "plugin/registration.go", "package plugin\n\nfunc init() {}\n")
	runGoAnalysisGit(t, root, "add", ".")
	runGoAnalysisGit(t, root, "commit", "-m", "registration")
	snapshot, err := repositoryfacts.BuildGitSnapshot(t.Context(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPackageToFileEdge(analysis, snapshot, "registers", "example.test/registration", "plugin/registration.go") {
		t.Fatalf("analysis omitted blank-import registration source: %#v", analysis.Edges)
	}
}

func TestAnalyzeIndexesExactEmbedBuildInput(t *testing.T) {
	root := newGoAnalysisRepository(t)
	writeGoAnalysisFile(t, root, "go.mod", "module example.test/embedinput\n\ngo 1.24\n")
	writeGoAnalysisFile(t, root, "source.go", `package embedinput

import _ "embed"

//go:embed assets/config.json
var config string
`)
	writeGoAnalysisFile(t, root, "assets/config.json", "{\"enabled\":true}\n")
	runGoAnalysisGit(t, root, "add", ".")
	runGoAnalysisGit(t, root, "commit", "-m", "embed input")
	snapshot, err := repositoryfacts.BuildGitSnapshot(t.Context(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPackageToFileEdge(analysis, snapshot, "embeds", "example.test/embedinput", "assets/config.json") {
		t.Fatalf("analysis omitted exact embedded configuration input: %#v", analysis.Edges)
	}
}

func TestAnalyzeIndexesOpaqueGoBuildInputWithoutGuessingItsReferences(t *testing.T) {
	root := newGoAnalysisRepository(t)
	writeGoAnalysisFile(t, root, "go.mod", "module example.test/opaqueinput\n\ngo 1.24\n")
	writeGoAnalysisFile(t, root, "source.go", "package opaqueinput\n\nfunc Source() {}\n")
	writeGoAnalysisFile(t, root, "source.s", "// exact opaque build input\n")
	runGoAnalysisGit(t, root, "add", ".")
	runGoAnalysisGit(t, root, "commit", "-m", "opaque input")
	snapshot, err := repositoryfacts.BuildGitSnapshot(t.Context(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := Analyze(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPackageToFileEdge(analysis, snapshot, "builds_from_opaque", "example.test/opaqueinput", "source.s") {
		t.Fatalf("analysis omitted exact opaque build input: %#v", analysis.Edges)
	}
}

func hasFileToSymbolEdge(
	analysis repositoryfacts.Analysis,
	snapshot repositoryfacts.Snapshot,
	kind, fromPath, toQualified string,
) bool {
	fromFileID := snapshotFileID(snapshot, fromPath)
	toID := analysisSymbolID(analysis, toQualified)
	for _, edge := range analysis.Edges {
		if edge.Kind == kind && artifactFileID(analysis, edge.FromID) == fromFileID && edge.ToID == toID {
			return true
		}
	}
	return false
}

func hasPackageToFileEdge(
	analysis repositoryfacts.Analysis,
	snapshot repositoryfacts.Snapshot,
	kind, fromPackage, toPath string,
) bool {
	fromID := analysisArtifactID(analysis, "go_package", fromPackage)
	toFileID := snapshotFileID(snapshot, toPath)
	for _, edge := range analysis.Edges {
		if edge.Kind == kind && edge.FromID == fromID && artifactFileID(analysis, edge.ToID) == toFileID {
			return true
		}
	}
	return false
}

func snapshotFileID(snapshot repositoryfacts.Snapshot, path string) string {
	for _, file := range snapshot.Files {
		if file.Path == path {
			return file.ID
		}
	}
	return ""
}

func analysisSymbolID(analysis repositoryfacts.Analysis, qualified string) string {
	for _, symbol := range analysis.Symbols {
		if symbol.QualifiedName == qualified {
			return symbol.ID
		}
	}
	return ""
}

func analysisArtifactID(analysis repositoryfacts.Analysis, kind, name string) string {
	for _, artifact := range analysis.Artifacts {
		if artifact.Kind == kind && artifact.Name == name {
			return artifact.ID
		}
	}
	return ""
}

func artifactFileID(analysis repositoryfacts.Analysis, artifactID string) string {
	for _, artifact := range analysis.Artifacts {
		if artifact.ID == artifactID {
			return artifact.FileID
		}
	}
	return ""
}
