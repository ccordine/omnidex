package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func executeChatCoreTurn(
	c *client.Client,
	input *chatInputReader,
	session string,
	baseMetadata map[string]any,
	lastJobID *int64,
	pendingInputs *[]string,
	instruction string,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	ui *chatUI,
) bool {
	turnMetadata := cloneMetadata(baseMetadata)
	turnMetadata["session_id"] = session
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		turnMetadata["client_cwd"] = cwd
		turnMetadata["host_env_cwd"] = cwd
	}
	applyHostTemporalMetadata(turnMetadata, time.Now())
	if *lastJobID > 0 {
		turnMetadata["parent_job_id"] = *lastJobID
	}

	job, err := c.Enqueue(context.Background(), instruction, model.PipelineChat, turnMetadata)
	if err != nil {
		emitAssistantError(ui, fmt.Sprintf("enqueue turn: %v", err))
		return false
	}
	*lastJobID = job.ID
	emitSystem(ui, fmt.Sprintf("assistant thinking (job %d)...", job.ID))
	emitSystem(ui, queuedTurnHintText())

	details, quit, err := awaitInteractiveTurn(c, input, job.ID, interval, progress, verbose, maxChars, pendingInputs, ui)
	if err != nil {
		emitAssistantError(ui, fmt.Sprintf("wait for turn: %v", err))
		return false
	}
	if quit {
		return true
	}

	if strings.TrimSpace(details.Job.Result) != "" {
		emitAssistant(ui, strings.TrimSpace(details.Job.Result))
	}
	if strings.TrimSpace(details.Job.Error) != "" {
		emitAssistantError(ui, strings.TrimSpace(details.Job.Error))
	}
	return false
}
