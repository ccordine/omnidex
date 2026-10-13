package codex

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

func publishAgentActivityText(activity model.AgentActivity, publish func(model.AgentActivity) error) error {
	text := activity.Text
	if !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
		return fmt.Errorf("public activity text is invalid UTF-8 or contains NUL")
	}
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
