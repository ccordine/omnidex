package workspace

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestWorkspaceMutationFenceExcludesOnlyTheSameDirectory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	first, err := AcquireMutationFence(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := TryAcquireMutationFence(ctx, root); !errors.Is(err, ErrWorkspaceBusy) {
		if second != nil {
			_ = second.Release()
		}
		t.Fatalf("same-directory acquisition: %v", err)
	}
	independent, err := TryAcquireMutationFence(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := independent.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err == nil {
		t.Fatal("released the same authority twice")
	}
	last, err := TryAcquireMutationFence(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := last.Release(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("directory lock created filesystem state: %v %v", entries, err)
	}
}

func TestWorkspaceMutationFenceIsReleasedWhenItsProcessEnds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, executable, "-test.run=^TestWorkspaceMutationFenceChildProcess$")
	command.Env = append(os.Environ(), "OMNI_TEST_FENCE_ROOT="+root)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	ready, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || ready != "acquired\n" {
		t.Fatalf("child readiness=%q: %v", ready, err)
	}
	if fence, err := TryAcquireMutationFence(ctx, root); !errors.Is(err, ErrWorkspaceBusy) {
		if fence != nil {
			_ = fence.Release()
		}
		t.Fatalf("separate process bypassed directory lock: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("terminated lock owner exited successfully")
	}
	acquired, err := AcquireMutationFence(ctx, root)
	if err != nil {
		t.Fatalf("dead process retained authority: %v", err)
	}
	if err := acquired.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceMutationFenceChildProcess(t *testing.T) {
	root := os.Getenv("OMNI_TEST_FENCE_ROOT")
	if root == "" {
		return
	}
	fence, err := AcquireMutationFence(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer fence.Release()
	fmt.Println("acquired")
	for {
		time.Sleep(time.Hour)
	}
}
