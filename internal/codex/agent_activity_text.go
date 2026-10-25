package codex

import (
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

func publishAgentActivityText(activity model.AgentActivity, publish func(model.AgentActivity) error) error {
	// Native evidence retains the original bytes. Its public display projection
	// must remain renderable even when a command prints binary or partial text.
	text := strings.ReplaceAll(strings.ToValidUTF8(activity.Text, "\uFFFD"), "\x00", "\uFFFD")
	for len(text) > model.MaxAgentActivityTextBytes {
		end := model.MaxAgentActivityTextBytes
		for !utf8.ValidString(text[:end]) {
			end--
		}
		chunk := activity
		chunk.Status, chunk.Text = model.AgentActivityDelta, text[:end]
		if err := publish(chunk); err != nil {
			return err
		}
		text = text[end:]
	}
	if text == "" && activity.Status == model.AgentActivityDelta {
		return nil
	}
	activity.Text = text
	return publish(activity)
}
