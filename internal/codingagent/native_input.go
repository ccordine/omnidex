package codingagent

import (
	"encoding/json"
	"fmt"
	"strings"
)

type NativeInput struct {
	Request  NativeInputRequest
	Response json.RawMessage
}

// Choices are created by the transport from the native request, never by an AI.
// The owner selects the response; Omnidex does not infer permission grants.
type NativeInputRequest struct {
	ID        json.RawMessage       `json:"id"`
	Method    string                `json:"method"`
	ThreadID  string                `json:"thread_id"`
	TurnID    string                `json:"turn_id"`
	Prompt    string                `json:"prompt"`
	Choices   []NativeInputChoice   `json:"choices,omitempty"`
	Questions []NativeInputQuestion `json:"questions,omitempty"`
	Form      json.RawMessage       `json:"form,omitempty"`
}

type NativeInputChoice struct {
	Label    string          `json:"label"`
	Response json.RawMessage `json:"response"`
}

type NativeInputQuestion struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Options []string `json:"options,omitempty"`
}

func (request NativeInputRequest) Reply(text string) (json.RawMessage, error) {
	for _, choice := range request.Choices {
		if strings.EqualFold(strings.TrimSpace(text), choice.Label) {
			return append(json.RawMessage(nil), choice.Response...), nil
		}
	}
	if len(request.Questions) > 0 {
		answers := map[string]struct {
			Answers []string `json:"answers"`
		}{}
		values := map[string]string{}
		if len(request.Questions) == 1 {
			values[request.Questions[0].ID] = text
		} else if err := json.Unmarshal([]byte(text), &values); err != nil {
			return nil, fmt.Errorf("reply with a JSON object mapping each question ID to its answer")
		}
		if len(values) != len(request.Questions) {
			return nil, fmt.Errorf("reply must answer each requested question exactly once")
		}
		for _, question := range request.Questions {
			answer, exists := values[question.ID]
			if !exists || strings.TrimSpace(answer) == "" {
				return nil, fmt.Errorf("missing answer for %s", question.ID)
			}
			answers[question.ID] = struct {
				Answers []string `json:"answers"`
			}{[]string{answer}}
		}
		return json.Marshal(map[string]any{"answers": answers})
	}
	if len(request.Form) > 0 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(text), &fields); err != nil || fields == nil {
			return nil, fmt.Errorf("reply with the requested form fields as a JSON object, or deny/cancel")
		}
		return json.Marshal(map[string]any{"action": "accept", "content": fields})
	}
	labels := make([]string, len(request.Choices))
	for index, choice := range request.Choices {
		labels[index] = choice.Label
	}
	return nil, fmt.Errorf("reply with exactly one of: %s", strings.Join(labels, ", "))
}
