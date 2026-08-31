package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type chatSessionConfig struct {
	Context  context.Context
	Cancel   context.CancelFunc
	Client   *client.Client
	Channel  model.Channel
	Snapshot client.ChatSessionSnapshot
	Stream   *client.JobEventStream
	Initial  string
	Input    io.Reader
	Output   io.Writer
	Errors   io.Writer
	Signals  <-chan os.Signal
}

type chatSession struct {
	ctx       context.Context
	client    *client.Client
	channel   model.Channel
	renderer  chatRenderer
	active    *model.JobDetails
	displayed map[int64]struct{}
}

func runChatSession(config chatSessionConfig) error {
	if config.Context == nil || config.Cancel == nil || config.Client == nil || config.Stream == nil {
		return fmt.Errorf("interactive chat requires context, client, and realtime stream")
	}
	session := &chatSession{
		ctx: config.Context, client: config.Client, channel: config.Channel,
		renderer:  chatRenderer{out: config.Output, err: config.Errors},
		displayed: make(map[int64]struct{}),
	}
	session.renderer.banner(config.Channel)
	session.renderer.transcript(config.Snapshot.Messages)
	if config.Snapshot.ActiveJob != nil {
		copy := *config.Snapshot.ActiveJob
		session.active = &copy
		session.renderer.system("reconnected to active job %d", copy.Job.ID)
		session.renderer.job(copy, nil)
	}

	inputs := readChatInput(config.Input)
	stream := followJobEvents(config.Context, config.Client, config.Stream)
	refresh := time.NewTicker(time.Second)
	defer refresh.Stop()
	if config.Initial != "" {
		if err := session.acceptText(config.Initial); err != nil {
			return err
		}
	}
	session.renderer.prompt(session.active)

	for {
		select {
		case <-config.Context.Done():
			return config.Context.Err()
		case signal := <-config.Signals:
			if signal == syscall.SIGTERM {
				config.Cancel()
				return nil
			}
			if session.active == nil {
				return nil
			}
			if err := session.control("interrupt", ctrlCInterruptReason); err != nil {
				session.renderer.system("interrupt failed: %v", err)
			} else {
				session.renderer.system("job %d interrupted; enter a redirection to resume", session.active.Job.ID)
			}
			session.renderer.prompt(session.active)
		case input, ok := <-inputs:
			if !ok || input.EOF {
				return nil
			}
			if input.Err != nil {
				return fmt.Errorf("read interactive input: %w", input.Err)
			}
			quit, err := session.acceptInput(input.Text)
			if err != nil {
				session.renderer.system("%v", err)
			}
			if quit {
				return nil
			}
			session.renderer.prompt(session.active)
		case update, ok := <-stream:
			if !ok {
				return fmt.Errorf("Omnidex realtime stream stopped")
			}
			if update.Err != nil {
				session.renderer.system("%v", update.Err)
				session.renderer.prompt(session.active)
				continue
			}
			if err := session.acceptRealtime(update.Event); err != nil {
				return err
			}
			session.renderer.prompt(session.active)
		case <-refresh.C:
			if session.active != nil {
		changed, err := session.refreshJob(false)
		if err != nil {
			return err
		}
		if changed {
			session.renderer.prompt(session.active)
		}
			}
		}
	}
}

func (session *chatSession) acceptInput(line string) (bool, error) {
	name, text, command := parseChatCommand(line)
	if !command {
		if strings.TrimSpace(text) == "" {
			return false, nil
		}
		return false, session.acceptText(text)
	}
	switch name {
	case "exit", "quit":
		return true, nil
	case "help":
		printChatHelp(session.renderer)
		return false, nil
	case "status":
		if session.active == nil {
			session.renderer.system("no active job")
			return false, nil
		}
		_, err := session.refreshJob(true)
		return false, err
	case "interrupt", "redirect", "replan", "feedback", "cancel":
		return false, session.control(name, text)
	default:
		return false, fmt.Errorf("unknown command /%s; use /help", name)
	}
}

func (session *chatSession) acceptText(text string) error {
	if session.active == nil {
		turn, err := session.client.SubmitChat(session.ctx, session.channel, text)
		if err != nil {
			return err
		}
		details := model.JobDetails{Job: turn.Job}
		session.active = &details
		session.renderer.system("submitted job %d", turn.Job.ID)
		_, err = session.refreshJob(true)
		return err
	}
	if session.active.Job.Status == model.JobStatusWaiting &&
		!interruptedBoundary(session.active.Steps) {
		return session.control("feedback", text)
	}
	return session.control("redirect", text)
}

func (session *chatSession) acceptRealtime(event client.RealtimeEvent) error {
	if event.EventName == client.RealtimeConnected {
		if event.SyncRequired {
			session.renderer.system("realtime replay gap; reloading persisted session state")
			return session.reloadSnapshot()
		}
		return nil
	}
	if event.ChannelID != "" && event.ChannelID != session.channel.ID {
		return nil
	}
	if session.active == nil {
		if event.ChannelID == session.channel.ID {
			return session.reloadSnapshot()
		}
		return nil
	}
	if event.JobID != session.active.Job.ID {
		return nil
	}
	session.renderer.realtime(event)
	_, err := session.refreshJob(false)
	return err
}

func (session *chatSession) reloadSnapshot() error {
	snapshot, err := session.client.ChatSession(session.ctx, session.channel, 100)
	if err != nil {
		return fmt.Errorf("reload CLI chat session: %w", err)
	}
	if snapshot.ActiveJob == nil {
		if session.active != nil {
			_, err := session.refreshJob(false)
			return err
		}
		return nil
	}
	if session.active != nil && session.active.Job.ID != snapshot.ActiveJob.Job.ID {
		return fmt.Errorf("session active job changed from %d to %d", session.active.Job.ID, snapshot.ActiveJob.Job.ID)
	}
	copy := *snapshot.ActiveJob
	previous := session.active
	session.active = &copy
	session.renderer.job(copy, previous)
	return nil
}

func (session *chatSession) refreshJob(force bool) (bool, error) {
	if session.active == nil {
		return false, nil
	}
	jobID := session.active.Job.ID
	details, err := session.client.Job(session.ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("refresh job %d: %w", jobID, err)
	}
	previous := *session.active
	session.active = &details
	changed := force || jobPresentationChanged(previous, details)
	if changed {
		session.renderer.job(details, &previous)
	}
	if !terminalJob(details.Job.Status) {
		return changed, nil
	}
	if _, shown := session.displayed[jobID]; !shown {
		session.renderer.terminal(details)
		session.displayed[jobID] = struct{}{}
	}
	session.active = nil
	return true, nil
}

func jobPresentationChanged(left, right model.JobDetails) bool {
	if left.Job.Status != right.Job.Status || left.Job.CurrentGeneration != right.Job.CurrentGeneration ||
		left.Job.Result != right.Job.Result || left.Job.Error != right.Job.Error || len(left.Steps) != len(right.Steps) {
		return true
	}
	for index := range left.Steps {
		if left.Steps[index].ID != right.Steps[index].ID ||
			left.Steps[index].Status != right.Steps[index].Status {
			return true
		}
	}
	return false
}

func terminalJob(status string) bool {
	return status == model.JobStatusCompleted || status == model.JobStatusFailed || status == model.JobStatusCanceled
}
