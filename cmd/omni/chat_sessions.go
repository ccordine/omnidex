package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/sessionlist"
	"github.com/gryph/omnidex/internal/workspacetransport"
)

type retainedChatSession struct {
	config     chatSessionConfig
	signals    chan os.Signal
	connection *workspacetransport.Local
	directory  *projectroot.DirectoryHandle
}

type chatSessions struct {
	ctx      context.Context
	clientID string
	terminal *sessionTerminal
	sessions map[model.ChannelID]*retainedChatSession
	selected model.ChannelID
	requests chan workspaceSessionRequest
	done     chan error
}

func runChatSessions(config chatSessionConfig, clientID string) (resultErr error) {
	ctx, cancel := context.WithCancel(config.Context)
	defer cancel()
	console := config.Console
	source := console.reviewInput.source.(*workspaceInputSource)
	terminal := &sessionTerminal{source: source.source, output: console.workspace.output, selected: config.Channel.ID}
	source.source = &sessionTerminalInput{terminal: terminal, id: config.Channel.ID, ctx: ctx}
	console.workspace.output = sessionTerminalOutput{terminal, config.Channel.ID}
	host := &chatSessions{ctx: ctx, clientID: clientID, terminal: terminal, sessions: make(map[model.ChannelID]*retainedChatSession), selected: config.Channel.ID, requests: make(chan workspaceSessionRequest, 1), done: make(chan error, 1)}
	config.Context, config.Cancel = context.WithCancel(ctx)
	first := &retainedChatSession{config: config, signals: make(chan os.Signal, 2)}
	host.add(first)
	completed := 0
	defer func() {
		cancel()
		// Wait for every session loop to retire its input reader before resources.
		for remaining := len(host.sessions) - completed; remaining > 0; remaining-- {
			resultErr = errors.Join(resultErr, <-host.done)
		}
		for id, entry := range host.sessions {
			if id == config.Channel.ID {
				continue
			}
			resultErr = errors.Join(resultErr, entry.config.Stream.Close())
			if entry.connection != nil {
				resultErr = errors.Join(resultErr, entry.connection.Close())
			}
			resultErr = errors.Join(resultErr, entry.directory.Close(), entry.config.Console.Close())
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-host.done:
			completed++
			return err
		case signal := <-config.Signals:
			select {
			case host.sessions[host.selected].signals <- signal:
			case <-ctx.Done():
				return ctx.Err()
			}
		case request := <-host.requests:
			if request.origin != host.selected {
				continue
			}
			entry := host.sessions[request.origin]
			if entry == nil {
				return fmt.Errorf("session request has no retained origin")
			}
			var err error
			if request.target == nil {
				err = host.refresh(entry)
			} else {
				err = host.selectSession(entry, *request.target)
			}
			if err != nil {
				view := entry.config.Console.workspace
				view.mu.Lock()
				view.sessions.errorText = safeConsoleText(err.Error())
				drawErr := view.drawLocked()
				view.mu.Unlock()
				if drawErr != nil {
					return drawErr
				}
			}
		}
	}
}

func (host *chatSessions) add(entry *retainedChatSession) {
	config := entry.config
	config.Signals = entry.signals
	entry.config = config
	view := config.Console.workspace
	view.mu.Lock()
	view.sessions.current, view.sessions.requests = config.Channel.ID, host.requests
	view.mu.Unlock()
	host.sessions[config.Channel.ID] = entry
	go func() {
		err := runChatSession(config)
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		host.done <- err
	}()
}

func (host *chatSessions) refresh(entry *retainedChatSession) error {
	listing, err := entry.config.Client.CLISessions(host.ctx, entry.config.Channel.WorkspaceRoot, entry.config.WorkspaceIdentity)
	if err != nil {
		return err
	}
	view := entry.config.Console.workspace
	view.mu.Lock()
	defer view.mu.Unlock()
	selected := view.sessions.current
	if i := view.sessions.selected; i >= 0 && i < len(view.sessions.entries) {
		selected = view.sessions.entries[i].ID
	}
	view.sessions.entries, view.sessions.errorText = listing.Sessions, ""
	view.sessions.selected = 0
	for i, item := range listing.Sessions {
		if item.ID == selected {
			view.sessions.selected = i
		}
	}
	return view.drawLocked()
}

func (host *chatSessions) selectSession(origin *retainedChatSession, target sessionlist.Entry) (resultErr error) {
	if err := target.Validate(host.clientID); err != nil {
		return err
	}
	entry := host.sessions[target.ID]
	if entry == nil {
		directory, err := projectroot.OpenDirectory(target.Directory)
		if err != nil {
			return err
		}
		keep := false
		defer func() {
			if !keep {
				resultErr = errors.Join(resultErr, directory.Close())
			}
		}()
		physical, err := directory.Identity()
		if err != nil {
			return err
		}
		identity, err := projectroot.ClientWorkspaceIdentity(host.clientID, physical)
		if err != nil {
			return err
		}
		if identity != target.WorkspaceIdentity {
			return fmt.Errorf("session directory has been replaced; original binding is unavailable")
		}
		ctx, cancel := context.WithCancel(host.ctx)
		defer func() {
			if !keep {
				cancel()
			}
		}()
		var connection *workspacetransport.Local
		if !target.Connected {
			connection, err = origin.config.Client.OpenWorkspace(ctx, directory, host.clientID)
			if err != nil {
				return err
			}
		}
		defer func() {
			if !keep && connection != nil {
				resultErr = errors.Join(resultErr, connection.Close())
			}
		}()
		bootstrap, err := loadChatBootstrap(ctx, ctx, origin.config.Client, target.Directory, identity)
		if err != nil {
			return err
		}
		defer func() {
			if !keep {
				resultErr = errors.Join(resultErr, bootstrap.stream.Close())
			}
		}()
		if bootstrap.channel.ID != target.ID {
			return fmt.Errorf("session bootstrap differs from listed origin")
		}
		view := origin.config.Console.workspace
		view.mu.Lock()
		width, height := view.width, view.height
		view.mu.Unlock()
		console, err := newRetainedSessionConsole(ctx, host.terminal, target.ID, width, height)
		if err != nil {
			return err
		}
		console.terminalFD = origin.config.Console.terminalFD
		entry = &retainedChatSession{config: chatSessionConfig{Context: ctx, Cancel: cancel, Client: origin.config.Client, Channel: bootstrap.channel, WorkspaceIdentity: identity, Snapshot: bootstrap.snapshot, Stream: bootstrap.stream, Console: console}, signals: make(chan os.Signal, 2), directory: directory, connection: connection}
		keep = true
		host.add(entry)
		if connection != nil {
			go func() {
				select {
				case <-connection.Done():
					cancel()
				case <-ctx.Done():
				}
			}()
		}
	}
	view := origin.config.Console.workspace
	view.mu.Lock()
	if view.focus == workspaceSessionsPane {
		view.focus = view.sessions.returnFocus
	}
	view.mu.Unlock()
	host.selected = target.ID
	return host.terminal.selectConsole(target.ID, entry.config.Console)
}
