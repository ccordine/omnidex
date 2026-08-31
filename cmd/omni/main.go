package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

const (
	defaultCoreURL = "https://omni.worknet"
	requestTimeout = 30 * time.Second
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "chat" {
		return fmt.Errorf("usage: omni chat [initial message]")
	}
	clientCWD, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("capture current working directory: %w", err)
	}
	if err := model.ValidateChannelWorkspaceRoot(clientCWD); err != nil {
		return fmt.Errorf("current working directory: %w", err)
	}
	coreURL := strings.TrimSpace(os.Getenv("CORE_URL"))
	if coreURL == "" {
		coreURL = defaultCoreURL
	}
	apiClient, err := client.New(coreURL, requestTimeout)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	stream, err := apiClient.OpenJobEvents(ctx, 0)
	if err != nil {
		return fmt.Errorf("open Omnidex realtime session: %w", err)
	}
	defer stream.Close()
	channel, err := apiClient.EnsureCLIChatChannel(ctx, clientCWD)
	if err != nil {
		return err
	}
	snapshot, err := apiClient.ChatSession(ctx, channel, 100)
	if err != nil {
		return fmt.Errorf("load CLI chat session: %w", err)
	}
	initial := strings.Join(args[1:], " ")
	if strings.TrimSpace(initial) == "" {
		initial = ""
	}
	return runChatSession(chatSessionConfig{
		Context: ctx, Cancel: cancel, Client: apiClient, Channel: channel,
		Snapshot: snapshot, Stream: stream, Initial: initial,
		Input: stdin, Output: stdout, Errors: stderr, Signals: signals,
	})
}
