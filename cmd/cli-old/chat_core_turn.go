package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func executeChatCoreTurn(
	c *client.Client,
	input *chatInputReader,
	channelID model.ChannelID,
	lastJobID *int64,
	pendingInputs *[]string,
	instruction string,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	ui *chatUI,
) bool {
	turn, err := c.PostChannelMessage(context.Background(), channelID, instruction)
	if err != nil {
		emitAssistantError(ui, fmt.Sprintf("post channel turn: %v", err))
		return false
	}
	job := turn.Job
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
