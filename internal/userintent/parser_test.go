package userintent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Scripted responses test the production envelope, typed validation and call
// boundaries. They are not evidence of a particular live model's language quality.
type testItem struct {
	ID       string `json:"id"`
	Approved bool   `json:"approved"`
}
type testState struct {
	JobID     int64      `json:"job_id"`
	PlanState string     `json:"plan_state"`
	JobStatus string     `json:"job_status"`
	Items     []testItem `json:"items"`
	Message   struct {
		Recipient string `json:"recipient"`
		Text      string `json:"text"`
	} `json:"message"`
}

func parserInput(t *testing.T, message string) Input {
	t.Helper()
	state := testState{JobID: 25, PlanState: "proposed", JobStatus: "waiting", Items: []testItem{{ID: "first"}, {ID: "established"}}}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return Input{Snapshot: raw, Prompt: message, Writable: func(path string) bool {
		switch path {
		case "/plan_state", "/job_status", "/items/0/approved", "/items/1/approved", "/message/recipient", "/message/text":
			return true
		}
		return false
	}, Validate: func(before, after json.RawMessage) error {
		old, err := DecodeState[testState](before)
		if err != nil {
			return err
		}
		next, err := DecodeState[testState](after)
		if err != nil {
			return err
		}
		if old.JobID != next.JobID || len(old.Items) != len(next.Items) {
			return fmt.Errorf("identity changed")
		}
		for i, item := range old.Items {
			if item.ID != next.Items[i].ID {
				return fmt.Errorf("item identity changed")
			}
		}
		if next.PlanState != "proposed" && next.PlanState != "approved" {
			return fmt.Errorf("invalid plan state")
		}
		if next.JobStatus != "waiting" && next.JobStatus != "running" {
			return fmt.Errorf("invalid job state")
		}
		if next.JobStatus == "running" && next.PlanState != "approved" {
			return fmt.Errorf("execution requires approval")
		}
		if next.Message.Recipient != "" && next.Message.Recipient != "codex" {
			return fmt.Errorf("invalid recipient")
		}
		if (next.Message.Text == "") != (next.Message.Recipient == "") {
			return fmt.Errorf("content requires recipient")
		}
		return nil
	}}
}
func TestDirectStatePatchSingleInterpretation(t *testing.T) {
	cases := []struct {
		name, message, response string
		changes                 int
	}{
		{"ordinary", "approve the plan", `{"patch":[{"op":"replace","path":"/plan_state","value":"approved"}]}`, 1},
		{"compound", "approve and run the plan", `{"patch":[{"op":"replace","path":"/plan_state","value":"approved"},{"op":"replace","path":"/job_status","value":"running"}]}`, 2},
		{"reference", "approve the second item in job 25", `{"patch":[{"op":"replace","path":"/items/1/approved","value":true}]}`, 1},
		{"recipient_negation_quotation", `Tell Codex to label the button "approve"; don't approve this plan.`, `{"patch":[{"op":"replace","path":"/message/recipient","value":"codex"},{"op":"replace","path":"/message/text","value":"label the button \"approve\"; don't approve this plan."}]}`, 2},
		{"compound_recipient", `approve the second item and tell Codex: preserve "cancel".`, `{"patch":[{"op":"replace","path":"/items/1/approved","value":true},{"op":"replace","path":"/message/recipient","value":"codex"},{"op":"replace","path":"/message/text","value":"preserve \"cancel\"."}]}`, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := parserInput(t, tc.message)
			input.Context = Context{Facts: []string{"The owner established job 25 and the second item."}, Recent: []Message{{"user", "Discuss the second item."}, {"assistant", "Its identity is established."}}}
			before := append([]byte(nil), input.Snapshot...)
			calls := 0
			result, err := Interpret(context.Background(), input, func(_ context.Context, prompt string) (string, error) {
				calls++
				for _, forbidden := range []string{"permitted_changes", "content_path", "alternatives", "callback", "Choose one"} {
					if strings.Contains(prompt, forbidden) {
						t.Fatalf("menu exposed: %s", forbidden)
					}
				}
				var envelope struct {
					Snapshot json.RawMessage `json:"server_snapshot"`
					Message  string          `json:"user_message"`
					Context  Context         `json:"context"`
				}
				at := strings.Index(prompt, "Input:\n")
				if at < 0 {
					t.Fatal("missing input")
				}
				if err := json.Unmarshal([]byte(prompt[at+len("Input:\n"):]), &envelope); err != nil {
					t.Fatal(err)
				}
				var original, rendered any
				json.Unmarshal(before, &original)
				json.Unmarshal(envelope.Snapshot, &rendered)
				if !reflect.DeepEqual(original, rendered) || envelope.Message != tc.message || !reflect.DeepEqual(input.Context, envelope.Context) {
					t.Fatal("input changed")
				}
				return tc.response, nil
			})
			if err != nil || calls != 1 || len(result.Patch) != tc.changes || result.Question != "" {
				t.Fatal(result, calls, err)
			}
			if string(input.Snapshot) != string(before) {
				t.Fatal("interpretation changed state")
			}
		})
	}
}
func TestClarificationHasNoEffects(t *testing.T) {
	input := parserInput(t, "remove that")
	validated := false
	input.Validate = func(json.RawMessage, json.RawMessage) error { validated = true; return nil }
	calls := 0
	result, err := Interpret(context.Background(), input, func(context.Context, string) (string, error) {
		calls++
		return `{"patch":[],"clarification":"Which item should be removed?"}`, nil
	})
	if err != nil || calls != 1 || validated || len(result.Patch) != 0 || result.Question == "" {
		t.Fatal(result, calls, validated, err)
	}
}
func TestInvalidPatchCannotEscapeStateContract(t *testing.T) {
	for _, raw := range []string{
		`{"patch":[{"op":"replace","path":"/plan_state","value":"proposed"}]}`,
		`{"patch":[{"op":"replace","path":"/job_id","value":26}]}`,
		`{"patch":[{"op":"replace","path":"/plan_state","value":true}]}`,
		`{"patch":[{"op":"replace","path":"/plan_state","value":"invented"}]}`,
		`{"patch":[{"op":"replace","path":"/job_status","value":"running"}]}`,
		`{"patch":[{"op":"replace","path":"/plan_state","value":"approved"},{"op":"replace","path":"/plan_state","value":"approved"}]}`,
		`{"patch":[{"op":"replace","path":"/message/recipient","value":"codex"}]}`,
		`{"patch":[{"op":"remove","path":"/items/9"}]}`,
		`{"patch":[{"op":"replace","path":"/plan_state","value":"approved"}],"clarification":"Which?"}`,
		`{"patch":[],"unexpected":true}`, `{"patch":[]} trailing`, `{"clarification":"Which?"}`,
	} {
		result, err := Interpret(context.Background(), parserInput(t, "change it"), func(context.Context, string) (string, error) { return raw, nil })
		if err == nil || len(result.Patch) > 0 {
			t.Fatalf("accepted %s: %+v %v", raw, result, err)
		}
	}
}
func TestCancellationAndModelFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	_, err := Interpret(ctx, parserInput(t, "approve"), func(context.Context, string) (string, error) { called = true; return "", nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatal(err, called)
	}
	failure := errors.New("model unavailable")
	_, err = Interpret(context.Background(), parserInput(t, "approve"), func(context.Context, string) (string, error) { return "", failure })
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

func TestLargeContextAddsOnlySharedMinificationBeforeInterpretation(t *testing.T) {
	input := parserInput(t, "approve the plan")
	input.Context = Context{Facts: []string{strings.Repeat("Relevant persisted information. ", 100)}}
	minifications, interpretations := 0, 0
	result, err := Interpret(context.Background(), input, func(_ context.Context, prompt string) (string, error) {
		if strings.HasPrefix(prompt, "What is the smallest context needed") {
			minifications++
			return "The established current plan is proposed.", nil
		}
		interpretations++
		if !strings.Contains(prompt, "established current plan") {
			t.Fatal("minified context missing")
		}
		return `{"patch":[{"op":"replace","path":"/plan_state","value":"approved"}]}`, nil
	})
	if err != nil || minifications != 1 || interpretations != 1 || len(result.Patch) != 1 {
		t.Fatal(result, minifications, interpretations, err)
	}
}
