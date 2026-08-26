package worker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/operation"
)

func TestDeploymentSnapshotBindsExactAssemblyBytesAfterLiveWorkspaceMutation(t *testing.T) {
	root, assembly := deploymentSnapshotFixture(t)
	identity, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, "generic-service-stack", assembly.VersionProfileID, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := directCodingCreateDeploymentWorkspaceSnapshot(root, identity, assembly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directCodingRemoveSnapshotStaging(snapshot.Root) })

	livePath := filepath.Join(root, "src", "Runtime.php")
	if err := os.WriteFile(livePath, []byte("<?php\n// unverified mutation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.VerifyExact(); err != nil {
		t.Fatalf("sealed snapshot followed mutable workspace bytes: %v", err)
	}
	snapshotContent, err := os.ReadFile(filepath.Join(snapshot.Root, "src", "Runtime.php"))
	if err != nil {
		t.Fatal(err)
	}
	if string(snapshotContent) != "<?php\nfinal class Runtime {}\n" ||
		!strings.HasSuffix(snapshot.Root, identity.WorkspaceSHA256) {
		t.Fatalf("snapshot root=%q content=%q", snapshot.Root, snapshotContent)
	}
}

func TestDeploymentSnapshotMutationBeforeBuildPreventsCommandSpawn(t *testing.T) {
	root, assembly := deploymentSnapshotFixture(t)
	identity, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, "generic-service-stack", assembly.VersionProfileID, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := directCodingCreateDeploymentWorkspaceSnapshot(root, identity, assembly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directCodingRemoveSnapshotStaging(snapshot.Root) })

	target := filepath.Join(snapshot.Root, "Dockerfile")
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("FROM unverified.invalid/runtime\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns := 0
	_, err = executeDirectCodingSnapshotBoundCommand(
		snapshot, snapshot.Root,
		func(string) (operation.Result, error) {
			spawns++
			return operation.Result{}, nil
		},
	)
	if err == nil || !errors.Is(err, errDirectCodingDeploymentSnapshotDrift) {
		t.Fatalf("mutated snapshot error=%v", err)
	}
	if spawns != 0 {
		t.Fatalf("mutated snapshot spawned %d deployment commands", spawns)
	}
}

func TestDeploymentSnapshotRejectsUnexpectedBuildInput(t *testing.T) {
	root, assembly := deploymentSnapshotFixture(t)
	identity, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, "generic-service-stack", assembly.VersionProfileID, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := directCodingCreateDeploymentWorkspaceSnapshot(root, identity, assembly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directCodingRemoveSnapshotStaging(snapshot.Root) })

	if err := os.Chmod(snapshot.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot.Root, "unverified.txt"), []byte("extra\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(snapshot.Root, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.VerifyExact(); err == nil || !errors.Is(err, errDirectCodingDeploymentSnapshotDrift) {
		t.Fatalf("unexpected build input error=%v", err)
	}
}

func TestDeploymentRecoveryReopensExactSnapshotWithoutReadingMutableWorkspace(t *testing.T) {
	root, assembly := deploymentSnapshotFixture(t)
	identity, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, "generic-service-stack", assembly.VersionProfileID, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := directCodingCreateDeploymentWorkspaceSnapshot(root, identity, assembly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directCodingRemoveSnapshotStaging(snapshot.Root) })
	if err := os.WriteFile(
		filepath.Join(root, "src", "Runtime.php"), []byte("<?php\n// later workspace generation\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	recovered, err := directCodingOpenDeploymentWorkspaceSnapshotFromAssembly(
		root, "generic-service-stack", assembly.VersionProfileID, assembly, identity,
	)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Root != snapshot.Root || recovered.Identity != identity {
		t.Fatalf("recovered=%+v snapshot=%+v", recovered, snapshot)
	}
}

func TestDeploymentRecoveryFailsWhenDurableSnapshotIsMissing(t *testing.T) {
	root, assembly := deploymentSnapshotFixture(t)
	identity, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, "generic-service-stack", assembly.VersionProfileID, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := directCodingCreateDeploymentWorkspaceSnapshot(root, identity, assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := directCodingRemoveSnapshotStaging(snapshot.Root); err != nil {
		t.Fatal(err)
	}
	if _, err := directCodingOpenDeploymentWorkspaceSnapshotFromAssembly(
		root, "generic-service-stack", assembly.VersionProfileID, assembly, identity,
	); err == nil || !errors.Is(err, errDirectCodingDeploymentSnapshotDrift) {
		t.Fatalf("missing recovered snapshot error=%v", err)
	}
	if _, err := os.Lstat(snapshot.Root); !os.IsNotExist(err) {
		t.Fatalf("recovery silently recreated missing snapshot: %v", err)
	}
}

func deploymentSnapshotFixture(t *testing.T) (string, directCodingAssembly) {
	t.Helper()
	root := t.TempDir()
	assembly := directCodingAssembly{
		VersionProfileID: "profile-v1",
		Files: []directCodingFileTask{
			{Path: ".dockerignore", Content: "**\n!src\n!src/Runtime.php\n"},
			{Path: "Dockerfile", Content: "FROM scratch\nCOPY src/Runtime.php /app/Runtime.php\n"},
			{Path: directCodingDeploymentComposePath, Content: "services:\n  app:\n    build: .\n"},
			{Path: "src/Runtime.php", Content: "<?php\nfinal class Runtime {}\n"},
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
	return root, assembly
}
