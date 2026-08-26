package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeploymentWorkspaceIdentityBindsExactVerifiedBytesAndTechnicalAuthority(t *testing.T) {
	root := t.TempDir()
	assembly := directCodingAssembly{
		VersionProfileID: "profile-v1",
		Files: []directCodingFileTask{
			{Path: "src/runtime.php", Content: "<?php\n"},
			{Path: directCodingDeploymentComposePath, Content: "services:\n  app:\n    image: example.invalid/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"},
		},
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range assembly.Files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file.Path)), []byte(file.Content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	first, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, "service-stack-a", assembly.VersionProfileID, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, "service-stack-a", assembly.VersionProfileID, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first.WorkspaceSHA256) != 64 || len(first.ComposeSHA256) != 64 || first.FileCount != 2 {
		t.Fatalf("identity=%+v repeated=%+v", first, second)
	}
	otherStack, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, "service-stack-b", assembly.VersionProfileID, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	if otherStack.WorkspaceSHA256 == first.WorkspaceSHA256 || otherStack.ComposeSHA256 != first.ComposeSHA256 {
		t.Fatal("workspace identity did not distinguish stack authority from structural Compose bytes")
	}
}

func TestDeploymentWorkspaceIdentityFailsOnPostVerificationMutation(t *testing.T) {
	root := t.TempDir()
	assembly := directCodingAssembly{
		VersionProfileID: "profile-v1",
		Files: []directCodingFileTask{
			{Path: directCodingDeploymentComposePath, Content: "services: {}\n"},
			{Path: "public/index.php", Content: "<?php\n"},
		},
	}
	for _, file := range assembly.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "public/index.php"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, "service-stack", assembly.VersionProfileID, assembly,
	); err == nil {
		t.Fatal("post-verification workspace mutation was accepted")
	}
}
