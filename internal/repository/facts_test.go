package repository

import (
	"context"
	"testing"
	"time"
)

func TestRepositoryArtifactsBindToExactSourceAuthority(t *testing.T) {
	root := newGitRepository(t)
	writeRepositoryTestFile(t, root, "main.go", "package main\n")
	runRepositoryGit(t, root, "add", "main.go")
	runRepositoryGit(t, root, "commit", "-m", "initial")
	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	file := snapshot.Files[0]
	adapter := AdapterIdentity{Name: "test", Version: "v1"}

	fileArtifact, err := NewFileArtifact(snapshot, file, "entry_point", "main", nil, OriginGoAST, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if fileArtifact.FileID != file.ID || fileArtifact.SourceSHA256 != file.SHA256 {
		t.Fatalf("file artifact=%+v file=%+v", fileArtifact, file)
	}
	snapshotArtifact, err := NewSnapshotArtifact(snapshot, "go_package", "example.test/main", nil, OriginGoPackages, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotArtifact.FileID != "" || snapshotArtifact.SourceSHA256 != snapshotContentSHA256(snapshot) {
		t.Fatalf("snapshot artifact=%+v", snapshotArtifact)
	}

	foreign := file
	foreign.ID = "file_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if _, err := NewFileArtifact(snapshot, foreign, "entry_point", "main", nil, OriginGoAST, adapter); err == nil {
		t.Fatal("file-bound artifact accepted a file outside its exact snapshot")
	}

	tampered := fileArtifact
	tampered.SourceSHA256 = snapshotContentSHA256(snapshot)
	analysis := Analysis{
		Schema: AnalysisSchemaV1, SnapshotID: snapshot.ID, Adapter: adapter,
		Complete: true, GeneratedAt: time.Now().UTC(), Artifacts: []Artifact{tampered},
		Symbols: []Symbol{}, Edges: []Edge{}, Diagnostics: []AnalysisDiagnostic{},
	}
	if err := FinalizeAnalysis(&analysis); err != nil {
		t.Fatal(err)
	}
	if err := analysis.Validate(snapshot); err == nil {
		t.Fatal("analysis accepted a file artifact bound to the snapshot hash instead of its file hash")
	}
}

func TestFinalizeAnalysisCanonicalizesGeneratedTimeForPersistence(t *testing.T) {
	analysis := Analysis{
		Schema:      AnalysisSchemaV1,
		GeneratedAt: time.Date(2026, time.August, 9, 12, 13, 14, 123456789, time.FixedZone("EDT", -4*60*60)),
		Symbols:     []Symbol{}, Artifacts: []Artifact{}, Edges: []Edge{}, Diagnostics: []AnalysisDiagnostic{},
	}
	if err := FinalizeAnalysis(&analysis); err != nil {
		t.Fatal(err)
	}
	if analysis.GeneratedAt.Location() != time.UTC || analysis.GeneratedAt.Nanosecond() != 123456000 {
		t.Fatalf("analysis generated time is not canonical: %#v", analysis.GeneratedAt)
	}
}

func TestAnalysisRejectsEdgesWithUnknownEndpoints(t *testing.T) {
	root := newGitRepository(t)
	writeRepositoryTestFile(t, root, "main.go", "package main\n")
	runRepositoryGit(t, root, "add", "main.go")
	runRepositoryGit(t, root, "commit", "-m", "initial")
	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	adapter := AdapterIdentity{Name: "test", Version: "v1"}
	artifact, err := NewSnapshotArtifact(
		snapshot, "package", "main", nil, OriginGoPackages, adapter,
	)
	if err != nil {
		t.Fatal(err)
	}
	edge := NewEdge(
		snapshot, adapter, artifact.ID,
		"symbol_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"contains", "", 0, 0, OriginGoPackages, 1,
	)
	analysis := Analysis{
		Schema: AnalysisSchemaV1, SnapshotID: snapshot.ID, Adapter: adapter,
		Complete: true, GeneratedAt: time.Now().UTC(),
		Symbols: []Symbol{}, Artifacts: []Artifact{artifact}, Edges: []Edge{edge},
		Diagnostics: []AnalysisDiagnostic{},
	}
	if err := FinalizeAnalysis(&analysis); err != nil {
		t.Fatal(err)
	}
	if err := analysis.Validate(snapshot); err == nil {
		t.Fatal("analysis accepted an edge whose endpoint is absent from exact facts")
	}
}
