package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspacetransport"
)

const (
	defaultCoreURL = "https://omni.worknet"
	requestTimeout = 30 * time.Second
)

type chatBootstrap struct {
	channel  model.Channel
	snapshot client.ChatSessionSnapshot
	stream   *client.JobEventStream
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) (resultErr error) {
	if len(args) == 2 && args[0] == "version" && args[1] == "--json" {
		return writeVersionJSON(stdout)
	}
	if len(args) != 1 || args[0] != "chat" {
		return fmt.Errorf("usage: omni chat | omni version --json")
	}
	if _, _, err := requireChatTerminalStreams(stdin, stdout); err != nil {
		return err
	}
	directory, err := projectroot.OpenInvokingDirectory()
	if err != nil {
		return fmt.Errorf("resolve current working directory authority: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	clientCWD := directory.Path()
	if err := model.ValidateChannelWorkspaceRoot(clientCWD); err != nil {
		return fmt.Errorf("current working directory: %w", err)
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate client configuration: %w", err)
	}
	clientID, err := loadClientInstallationIdentity(configRoot)
	if err != nil {
		return err
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

	workspaceConnection, err := awaitChatRequest(ctx, signals,
		func(context.Context) (*workspacetransport.Local, error) {
			// The connection belongs to the session context. Completing this
			// interruptible startup request must not close that connection.
			return apiClient.OpenWorkspace(ctx, directory, clientID)
		},
	)
	if errors.Is(err, errChatRequestInterrupted) || errors.Is(err, errChatTerminated) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, workspaceConnection.Close()) }()
	workspaceIdentity := workspaceConnection.Identity()
	go func() {
		select {
		case <-workspaceConnection.Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	bootstrap, err := awaitChatRequest(
		ctx,
		signals,
		func(requestContext context.Context) (chatBootstrap, error) {
			return loadChatBootstrap(requestContext, ctx, apiClient, clientCWD, workspaceIdentity)
		},
	)
	if errors.Is(err, errChatRequestInterrupted) || errors.Is(err, errChatTerminated) {
		return nil
	}
	if err != nil {
		return err
	}
	defer bootstrap.stream.Close()
	console, err := newChatConsole(ctx, stdin, stdout, stderr)
	if err != nil {
		return err
	}
	sessionErr := runChatSession(chatSessionConfig{
		Context: ctx, Cancel: cancel, Client: apiClient, Channel: bootstrap.channel,
		WorkspaceIdentity: workspaceIdentity,
		Snapshot:          bootstrap.snapshot,
		Stream:            bootstrap.stream,
		Console:           console,
		Signals:           signals,
	})
	return errors.Join(sessionErr, workspaceConnection.Err(), console.Close())
}

func loadChatBootstrap(
	ctx context.Context,
	lifetime context.Context,
	apiClient *client.Client,
	clientCWD string,
	workspaceIdentity string,
) (chatBootstrap, error) {
	channel, err := apiClient.BootstrapCLIChatSession(ctx, clientCWD, workspaceIdentity)
	if err != nil {
		return chatBootstrap{}, err
	}
	stream, err := apiClient.OpenJobEvents(lifetime, channel.ID, workspaceIdentity, nil)
	if err != nil {
		return chatBootstrap{}, fmt.Errorf("open Omnidex realtime session: %w", err)
	}
	snapshot, err := apiClient.ChatSession(
		ctx,
		channel,
		workspaceIdentity,
		client.MaxChatSessionMessages,
	)
	if err != nil {
		return chatBootstrap{}, errors.Join(
			fmt.Errorf("load CLI chat session: %w", err),
			stream.Close(),
		)
	}
	return chatBootstrap{channel: channel, snapshot: snapshot, stream: stream}, nil
}
