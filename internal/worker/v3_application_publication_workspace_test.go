package worker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestPublishedFilesAreVerifiedWithoutRewriting(t *testing.T) {
	session := publicationWorkspaceFixture(t)
	assembly := publicationWorkspaceAssembly()
	for iteration := 0; iteration < 2; iteration++ {
		prepared, err := session.prepareWorkspaceReconciliation(assembly)
		if err != nil {
			t.Fatal(err)
		}
		if err := session.ApplyAndVerify(prepared); err != nil {
			t.Fatal(err)
		}
		if iteration == 1 && len(prepared.result.Changes) != 0 {
			t.Fatal("unchanged published files were rewritten")
		}
	}
	if len(session.publishedAssembly.Files) != 2 || len(session.mutationJournal) != 2 {
		t.Fatalf("publication did not retain exact observed files: %+v", session.mutationJournal)
	}
}

func TestPublicationPreservesEditsMadeAfterPreparation(t *testing.T) {
	session := publicationWorkspaceFixture(t)
	assembly := publicationWorkspaceAssembly()
	first := directCodingAssembly{Files: assembly.Files[:1]}
	prepared, err := session.prepareWorkspaceReconciliation(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err != nil {
		t.Fatal(err)
	}
	prepared, err = session.prepareWorkspaceReconciliation(assembly)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(session.root, "a.json")
	if err := os.WriteFile(path, []byte("user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err == nil || !strings.Contains(err.Error(), "previously published workspace changed") {
		t.Fatalf("changed publication was overwritten: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "user edit\n" {
		t.Fatalf("external edit lost: %q %v", content, err)
	}
	if _, err := os.Lstat(filepath.Join(session.root, "b.txt")); !os.IsNotExist(err) {
		t.Fatal("publication continued after its accepted base changed")
	}
}

func TestPartialPublicationRetainsEachVerifiedWrite(t *testing.T) {
	session := publicationWorkspaceFixture(t)
	assembly := publicationWorkspaceAssembly()
	prepared, err := session.prepareWorkspaceReconciliation(assembly)
	if err != nil {
		t.Fatal(err)
	}
	conflict := filepath.Join(session.root, "b.txt")
	if err := os.WriteFile(conflict, []byte("unowned\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err == nil {
		t.Fatal("create-only publication replaced an unowned file")
	}
	if len(session.publishedAssembly.Files) != 1 || session.publishedAssembly.Files[0].Path != "a.json" {
		t.Fatal("successful write was discarded after another file failed")
	}
	content, err := os.ReadFile(conflict)
	if err != nil || string(content) != "unowned\n" {
		t.Fatalf("unowned file changed: %q %v", content, err)
	}
	if err := os.Remove(conflict); err != nil {
		t.Fatal(err)
	}
	prepared, err = session.prepareWorkspaceReconciliation(assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err != nil {
		t.Fatal(err)
	}
	if len(prepared.result.Changes) != 1 || prepared.result.Changes[0].Path != "b.txt" {
		t.Fatalf("remaining publication revisited accepted state: %+v", prepared.result.Changes)
	}
}

func TestPublicationRejectsChangedOrOmittedAcceptedFiles(t *testing.T) {
	for _, defect := range []string{"content", "permissions", "omission"} {
		published := publicationWorkspaceAssembly()
		next := cloneDirectCodingAssembly(published)
		switch defect {
		case "content":
			next.Files[0].Content = []byte("different")
		case "permissions":
			next.Files[0].Mode = 0o600
		case "omission":
			next.Files = next.Files[1:]
		}
		if _, err := directCodingUnpublishedAssembly(next, published); err == nil {
			t.Fatalf("accepted publication allowed %s", defect)
		}
	}
}

func TestPublicationUsesProtectedMoveSourceAsContentOnly(t *testing.T) {
	session := publicationWorkspaceFixture(t)
	session.program = nil
	session.protectedPaths = map[string]struct{}{"original.txt": {}}
	content := []byte("preserved source\n")
	if err := os.WriteFile(filepath.Join(session.root, "original.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	assembly := directCodingAssembly{Files: []directCodingFileTask{{
		Path: "copy.txt", Content: content, Mode: 0o644, MoveFrom: "original.txt",
	}}}
	prepared, err := session.PrepareAssembly(assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err != nil {
		t.Fatal(err)
	}
	prepared, err = session.PrepareAssembly(assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyAndVerify(prepared); err != nil || len(prepared.result.Changes) != 0 {
		t.Fatalf("protected copy replay changed accepted state: %v %+v", err, prepared.result.Changes)
	}
	for _, path := range []string{"original.txt", "copy.txt"} {
		actual, err := os.ReadFile(filepath.Join(session.root, path))
		if err != nil || string(actual) != string(content) {
			t.Fatalf("protected copy changed %s: %q %v", path, actual, err)
		}
	}
}

func publicationWorkspaceAssembly() directCodingAssembly {
	return directCodingAssembly{Files: []directCodingFileTask{
		{Path: "a.json", Content: []byte("{\"value\":1}\n"), Mode: 0o644},
		{Path: "b.txt", Content: []byte("second artifact\n"), Mode: 0o644},
	}}
}

// This fixture exercises the existing reconciler and create-only policy;
// source validation and actual compiler evidence have separate tests.
func publicationWorkspaceFixture(t *testing.T) *directCodingSession {
	t.Helper()
	root := t.TempDir()
	fence, err := workspacefacts.AcquireMutationFence(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fence.Release(); err != nil {
			t.Error(err)
		}
	})
	metadata, err := json.Marshal(map[string]string{"client_cwd": root})
	if err != nil {
		t.Fatal(err)
	}
	return &directCodingSession{
		root: root, program: &directCodingProgram{Project: directCodingProjectSelection{Stack: directCodingProjectStack{ID: genericTypeScriptBrowserAdapter}}},
		runtime: &nativeRuntimeV3{
			ctx: context.Background(), claim: &model.ClaimedStep{Job: model.Job{Metadata: metadata}},
			svc:            &Service{hostDirectoryAccess: workspacefacts.NewHostDirectoryAccess(root)},
			workspaceFence: fence, workspaceFenceRoot: root,
		},
	}
}
