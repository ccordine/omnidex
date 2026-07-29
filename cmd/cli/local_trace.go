package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type localExecutionTraceSink func(string)

var (
	localExecutionTraceMu   sync.RWMutex
	localExecutionTraceHook localExecutionTraceSink
)

func installLocalExecutionTraceSink(sink localExecutionTraceSink) func() {
	localExecutionTraceMu.Lock()
	previous := localExecutionTraceHook
	localExecutionTraceHook = sink
	localExecutionTraceMu.Unlock()
	return func() {
		localExecutionTraceMu.Lock()
		localExecutionTraceHook = previous
		localExecutionTraceMu.Unlock()
	}
}

func emitLocalExecutionTrace(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	localExecutionTraceMu.RLock()
	sink := localExecutionTraceHook
	localExecutionTraceMu.RUnlock()
	if sink != nil {
		sink(trimmed)
		return
	}

	if strings.TrimSpace(os.Getenv("OMNI_FRONTEND_TRACE")) != "" {
		fmt.Fprintf(os.Stderr, "TRACE> %s\n", trimmed)
	}
}

func traceLocalCommandInvocation(args ...string) {
	if len(args) == 0 {
		return
	}
	emitLocalExecutionTrace("frontend exec | command=" + formatTraceCommand(args))
}

func formatTraceCommand(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, quoteTraceArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteTraceArg(arg string) string {
	if arg == "" {
		return `""`
	}
	for _, r := range arg {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("._/:=@%+,-", r):
		default:
			return strconv.Quote(arg)
		}
	}
	return arg
}

func tracedExecCommand(name string, args ...string) *exec.Cmd {
	traceLocalCommandInvocation(append([]string{name}, args...)...)
	return exec.Command(name, args...)
}

func tracedExecCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	traceLocalCommandInvocation(append([]string{name}, args...)...)
	return exec.CommandContext(ctx, name, args...)
}
