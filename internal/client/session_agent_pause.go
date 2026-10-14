package client

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func validateChatSessionAgentPause(details model.JobDetails, execution *codingagent.Configuration) error {
	pause := details.AgentPause
	if pause == nil {
		return nil
	}
	if execution == nil || execution.Mode != codingagent.Agents || details.Job.Status != model.JobStatusWaiting {
		return fmt.Errorf("chat session agent question lacks a waiting Agents job")
	}
	waitingStepID := int64(0)
	for _, step := range details.Steps {
		if step.Status != model.StepStatusWaiting {
			continue
		}
		if (step.Action != "v3_coding_plan" && step.Action != "v3_coding") || waitingStepID != 0 {
			return fmt.Errorf("chat session agent question lacks one exact waiting planning or coding step")
		}
		waitingStepID = step.ID
	}
	if waitingStepID == 0 ||
		pause.Generation != details.Job.CurrentGeneration || pause.StepID != waitingStepID ||
		pause.WaitingAttempt < 1 {
		return fmt.Errorf("chat session agent question differs from its exact waiting authority")
	}
	return validateAgentQuestion(pause)
}

func validateAgentQuestion(pause *model.CodingAgentPause) error {
	if pause == nil || pause.Schema != "omnidex-coding-agent-pause-v1" ||
		pause.Generation < 1 || pause.StepID < 1 || pause.WaitingAttempt < 1 || len(pause.Questions) == 0 {
		return fmt.Errorf("agent question lacks its exact waiting authority")
	}
	for _, question := range pause.Questions {
		if strings.TrimSpace(question) == "" || !utf8.ValidString(question) || strings.ContainsRune(question, '\x00') {
			return fmt.Errorf("chat session agent pause contains an invalid question")
		}
	}
	return nil
}
