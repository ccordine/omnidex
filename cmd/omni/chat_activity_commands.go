package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func (session *chatSession) setChatDiagnostics(text string) error {
	switch strings.TrimSpace(text) {
	case "on":
		session.renderer.verbosity = chatDebug
	case "off":
		session.renderer.verbosity = chatNormal
	case "":
	default:
		return fmt.Errorf("use /diagnostics on or /diagnostics off")
	}
	label := "off"
	if session.renderer.verbosity == chatDebug {
		label = "on"
	}
	return session.renderer.system("Diagnostics %s", label)
}

func (session *chatSession) renderTurnAccepted(receipt client.SessionTurnReceipt) error {
	if session.renderer.verbosity == chatDebug {
		return session.renderer.system("job %d · %s", receipt.JobID, receipt.Disposition)
	}
	label := "Request accepted."
	switch receipt.Disposition {
	case model.ChannelSessionTurnReplanned:
		label = "Plan updated."
	case model.ChannelSessionTurnFeedback:
		label = "Follow-up received."
	case model.ChannelSessionTurnQueued:
		label = "Message queued."
	}
	return session.renderer.console.WriteOutput("omni> " + label + "\n")
}

func (session *chatSession) setChatVerbosity(text string) error {
	switch strings.TrimSpace(text) {
	case "quiet":
		session.renderer.verbosity = chatQuiet
	case "normal":
		session.renderer.verbosity = chatNormal
	case "verbose":
		session.renderer.verbosity = chatVerbose
	case "debug":
		session.renderer.verbosity = chatDebug
	case "":
	default:
		return fmt.Errorf("use /verbosity quiet, normal, verbose, or debug")
	}
	return session.renderer.system("Verbosity: %s", session.renderer.verbosity)
}

func (session *chatSession) stopCurrentJob(reason string) error {
	jobID := int64(0)
	if session.active != nil {
		jobID = session.active.Job.ID
	}
	_, err := session.control("cancel", reason, true, &jobID)
	if errors.Is(err, errNoActiveChatJob) {
		return session.renderer.system("No active task.")
	}
	return err
}
