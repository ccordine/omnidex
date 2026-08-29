package changeapply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestVerifyExactDeltaRejectsTamperAndClosedStage(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	stage, err := fixture.plan(
		fixture.contract(t, "First"),
		map[string]string{
			fixture.symbol(t, "First").ID: "func First() int { return 2 }",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.VerifyExactDelta(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(stage.DeltaRoot(), "first.go"),
		[]byte("package changeapply\n\nfunc First() int { return 9 }\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := stage.VerifyExactDelta(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "tampered") {
		t.Fatalf("tampered stage verification error=%v", err)
	}
	if err := stage.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := stage.VerifyExactDelta(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "closed") {
		t.Fatalf("closed stage verification error=%v", err)
	}
}

func TestVerifyExactDeltaHonorsCanceledAuthority(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (*changeapply.StagedChange)(nil).VerifyExactDelta(ctx); err == nil ||
		!strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("canceled exact stage verification error=%v", err)
	}
}
