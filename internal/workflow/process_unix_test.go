//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package workflow

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestToolRemainsInOwningNativeCommandGroup(t *testing.T) {
	scope := fixtureScope(t)
	var err error
	scope.Go, err = exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Execute(t.Context(), scope, "go", []string{"-c", "ps -o pgid= -p $$"}, nil, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != fmt.Sprint(syscall.Getpgrp()) {
		t.Fatal("tool detached from native cancellation ownership", output.String())
	}
}
