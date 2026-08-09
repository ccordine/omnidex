package repository

import (
	"context"
	"strings"
	"testing"
)

func TestReadExactSymbolSpanReturnsPathFreeHashCheckedSource(t *testing.T) {
	root := newGitRepository(t)
	source := "package sample\n\nfunc Value() int { return 1 }\n"
	writeRepositoryTestFile(t, root, "sample.go", source)
	runRepositoryGit(t, root, "add", "sample.go")
	runRepositoryGit(t, root, "commit", "-m", "source")
	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	file := snapshot.Files[0]
	start := int64(strings.Index(source, "func Value"))
	end := int64(len(source) - 1)
	symbol := NewSymbol(snapshot, file, AdapterIdentity{Name: "go", Version: "test"},
		"function", "Value", "example.Value", "func Value() int", start, end, OriginGoAST, 1)
	span, err := ReadExactSymbolSpan(snapshot, symbol, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if span.SubjectID != symbol.ID || span.FileID != file.ID || span.SourceSHA256 != file.SHA256 {
		t.Fatalf("span identity=%#v", span)
	}
	if span.Content != "func Value() int { return 1 }" {
		t.Fatalf("content=%q", span.Content)
	}
}

func TestReadExactSymbolSpanRejectsStaleOrOversizedAuthority(t *testing.T) {
	root := newGitRepository(t)
	source := "package sample\n\nfunc Value() int { return 1 }\n"
	writeRepositoryTestFile(t, root, "sample.go", source)
	runRepositoryGit(t, root, "add", "sample.go")
	runRepositoryGit(t, root, "commit", "-m", "source")
	snapshot, err := BuildGitSnapshot(context.Background(), root, SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	file := snapshot.Files[0]
	start := int64(strings.Index(source, "func Value"))
	symbol := NewSymbol(snapshot, file, AdapterIdentity{Name: "go", Version: "test"},
		"function", "Value", "example.Value", "func Value() int", start, int64(len(source)-1), OriginGoAST, 1)
	if _, err := ReadExactSymbolSpan(snapshot, symbol, 4); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized span error=%v", err)
	}
	writeRepositoryTestFile(t, root, "sample.go", strings.ReplaceAll(source, "return 1", "return 2"))
	if _, err := ReadExactSymbolSpan(snapshot, symbol, 4096); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale span error=%v", err)
	}
}
