package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const directCodingProcessGroupTestRole = "OMNIDEX_PROCESS_GROUP_TEST_ROLE"

func TestVerificationProcessCancellationReapsDescendantBeforeReturn(t *testing.T) {
	switch os.Getenv(directCodingProcessGroupTestRole) {
	case "parent":
		runDirectCodingProcessGroupParentHelper(t)
		return
	case "descendant":
		runDirectCodingProcessGroupDescendantHelper(t)
		return
	}

	pidPath := t.TempDir() + "/descendant.pid"
	process := exec.Command(
		os.Args[0],
		"-test.run=^TestVerificationProcessCancellationReapsDescendantBeforeReturn$",
	)
	process.Env = append(
		os.Environ(),
		directCodingProcessGroupTestRole+"=parent",
		"OMNIDEX_PROCESS_GROUP_PID_PATH="+pidPath,
	)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		finished <- runDirectCodingVerificationProcess(ctx, process)
	}()

	descendantPID := awaitDirectCodingProcessGroupDescendantPID(t, pidPath)
	cancel()
	select {
	case <-finished:
	case <-time.After(4 * time.Second):
		t.Fatal("verification process-tree cancellation did not return within its bound")
	}
	if err := syscall.Kill(descendantPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("descendant %d remained after command return: %v", descendantPID, err)
	}
}

func runDirectCodingProcessGroupParentHelper(t *testing.T) {
	terminated := make(chan os.Signal, 1)
	signal.Notify(terminated, syscall.SIGTERM)
	defer signal.Stop(terminated)
	child := exec.Command(
		os.Args[0],
		"-test.run=^TestVerificationProcessCancellationReapsDescendantBeforeReturn$",
	)
	child.Env = append(os.Environ(), directCodingProcessGroupTestRole+"=descendant")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-terminated:
	case <-time.After(10 * time.Second):
		t.Fatal("parent helper did not receive process-group termination")
	}
	if err := child.Wait(); err != nil {
		var exitErr *exec.ExitError
		status, statusOK := exitErrProcessStatus(err, &exitErr)
		if !statusOK || !status.Signaled() || status.Signal() != syscall.SIGTERM {
			t.Fatalf("wait terminated descendant: %v", err)
		}
	}
}

func exitErrProcessStatus(err error, exitErr **exec.ExitError) (syscall.WaitStatus, bool) {
	if !errors.As(err, exitErr) || *exitErr == nil || (*exitErr).ProcessState == nil {
		return 0, false
	}
	status, ok := (*exitErr).ProcessState.Sys().(syscall.WaitStatus)
	return status, ok
}

func runDirectCodingProcessGroupDescendantHelper(t *testing.T) {
	pidPath := os.Getenv("OMNIDEX_PROCESS_GROUP_PID_PATH")
	if strings.TrimSpace(pidPath) == "" {
		t.Fatal("descendant helper PID path is unavailable")
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	select {}
}

func awaitDirectCodingProcessGroupDescendantPID(t *testing.T, pidPath string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		value, err := os.ReadFile(pidPath)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(value)))
			if parseErr != nil || pid <= 0 {
				t.Fatalf("parse descendant PID %q: %v", value, parseErr)
			}
			return pid
		}
		if !os.IsNotExist(err) {
			t.Fatalf("read descendant PID: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("descendant helper did not publish its PID")
	return 0
}
