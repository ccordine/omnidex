package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthoritativeRepositoryPostRequiresExactAppliedInventory(t *testing.T) {
	t.Parallel()
	before, contract, commands, prepared := repositoryMutationExecutionFixture(t)
	t.Cleanup(func() { _ = prepared.Cleanup() })
	assertPost := func() error {
		return assertExactAuthoritativeRepositoryPost(
			context.Background(), before.Root, before, contract.ID, commands, prepared,
		)
	}
	if err := assertPost(); err == nil || !strings.Contains(err.Error(), "exact verified post") {
		t.Fatalf("source state was accepted as authoritative post: %v", err)
	}
	if _, err := prepared.ApplyVerified(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := assertPost(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(before.Root, "first.go"),
		[]byte("package verification\n\nfunc First() int { return 9 }\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := assertPost(); err == nil || !strings.Contains(err.Error(), "exact verified post") {
		t.Fatalf("tampered authoritative state error=%v", err)
	}
}

func TestAuthoritativeRepositoryPostRejectsDifferentRoot(t *testing.T) {
	t.Parallel()
	before, contract, commands, prepared := repositoryMutationExecutionFixture(t)
	t.Cleanup(func() { _ = prepared.Cleanup() })
	err := assertExactAuthoritativeRepositoryPost(
		context.Background(), t.TempDir(), before, contract.ID, commands, prepared,
	)
	if err == nil || !strings.Contains(err.Error(), "root differs") {
		t.Fatalf("wrong authoritative root error=%v", err)
	}
}
