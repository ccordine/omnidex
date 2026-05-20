package odn

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLiveOllamaFindsVirginiaTimeFromCommandEvidence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live time test in short mode")
	}
	skipUnlessLiveOllamaEnabled(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute

	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	baseline := time.Now().In(location)
	baselineHour := baseline.Hour()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecision(ctx, "What time is it in Virginia right now?", client, stdout, stderr)
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during live time test: %v", err)
		}
		t.Fatal(err)
	}
	if !hasRealCommandObservation(result.Observations) {
		t.Fatalf("expected real command observation: %#v", result.Observations)
	}

	evidence := strings.Join([]string{stdout.String(), stderr.String(), result.Answer}, "\n")
	assertNoFalseCapabilityLimitation(t, client, result, stdout.String(), stderr.String())
	if !virginiaTimeEvidenceMatches(evidence, baselineHour) {
		t.Fatalf(
			"ODN time evidence did not overlap Virginia baseline hour\nbaseline_hour=%02d\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s",
			baselineHour,
			result.Command,
			result.Answer,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestLiveOllamaFindsPattayaTimeWithoutFalseCapabilityLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Pattaya time loop regression in short mode")
	}
	skipUnlessLiveOllamaEnabled(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute

	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	baseline := time.Now().In(location)
	baselineHour := baseline.Hour()

	history := []Message{
		{Role: "user", Content: "what's the weather in Pattaya right now?"},
		{Role: "assistant", Content: "Command: curl -s 'https://wttr.in/Pattaya?format=%l|%C|%t|%f'\nAnswer: Pattaya weather evidence was gathered from wttr.in."},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecisionWithHistoryEventsAndAsk(
		ctx,
		"Okay what time is it in Pattaya right now?",
		history,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
	)
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during live Pattaya time test: %v", err)
		}
		t.Fatalf("Pattaya time lookup failed: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v\nevents=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations, events)
	}
	if !hasSuccessfulCommandObservation(result.Observations) {
		t.Fatalf("expected successful command evidence\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v\nevents=%#v",
			result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations, events)
	}
	if structuredEventsContain(events, "structured_loop_exhausted") {
		t.Fatalf("structured loop exhausted despite local timezone capability\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v\nevents=%#v",
			result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations, events)
	}
	if countStructuredEvents(events, "structured_llm_request_started") > 5 {
		t.Fatalf("too many planner retries for a direct timezone lookup\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v\nevents=%#v",
			result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations, events)
	}

	evidence := strings.Join([]string{stdout.String(), stderr.String(), result.Answer}, "\n")
	assertNoFalseCapabilityLimitation(t, client, result, stdout.String(), stderr.String())
	review := reviewModelOutputForBlindCannotDoClaim(t, client, result.Answer)
	if review.ClaimedCannotDo {
		t.Fatalf("blind LLM reviewer found a false cannot-do claim without seeing the prompt\nreview=%#v\nanswer=%q\ncommand=%q\nstdout=%s\nstderr=%s",
			review, result.Answer, result.Command, stdout.String(), stderr.String())
	}
	if !pattayaTimeEvidenceMatches(evidence, baselineHour) {
		t.Fatalf(
			"ODN Pattaya time evidence did not overlap Thailand baseline hour\nbaseline_hour=%02d\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			baselineHour,
			result.Command,
			result.Answer,
			stdout.String(),
			stderr.String(),
			result.Observations,
		)
	}
}

func TestLiveOllamaShellSpecialistHandlesDelegatedPattayaTime(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live shell specialist delegation regression in short mode")
	}
	skipUnlessLiveOllamaEnabled(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute

	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	baselineHour := time.Now().In(location).Hour()

	planner := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"Get the current local time in Pattaya, Thailand using system timezone evidence."}`,
		`{"command":"","done":true,"answer":""}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := runStructuredCommandDecisionWithConfig(
		ctx,
		"What time is it in Pattaya right now?",
		nil,
		planner,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			ShellSpecialist: NewOllamaShellCommandSpecialist(client),
		},
	)
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during live shell specialist test: %v", err)
		}
		t.Fatalf("delegated Pattaya time lookup failed: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v\nevents=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations, events)
	}
	if !structuredEventsContain(events, "structured_tool_delegation_started") || !structuredEventsContain(events, "structured_tool_delegation_finished") {
		t.Fatalf("missing shell delegation events\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nevents=%#v",
			result.Command, result.Answer, stdout.String(), stderr.String(), events)
	}
	evidence := strings.Join([]string{stdout.String(), stderr.String(), result.Answer}, "\n")
	assertNoFalseCapabilityLimitation(t, client, result, stdout.String(), stderr.String())
	if !pattayaTimeEvidenceMatches(evidence, baselineHour) {
		t.Fatalf("delegated Pattaya time evidence did not overlap Thailand baseline hour\nbaseline_hour=%02d\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			baselineHour, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
	}
}

type blindCapabilityReview struct {
	ClaimedCannotDo bool   `json:"claimed_cannot_do"`
	Evidence        string `json:"evidence"`
}

func reviewModelOutputForBlindCannotDoClaim(t *testing.T, client *OllamaClient, modelOutput string) blindCapabilityReview {
	t.Helper()
	if strings.TrimSpace(modelOutput) == "" {
		t.Fatal("model output is empty; blind cannot-do review would be meaningless")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resp, err := client.ChatRaw(ctx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are a strict blind reviewer.",
					"You do not know the user's prompt.",
					"You only judge whether the supplied model_output claims the assistant cannot do something.",
					"Return JSON only with schema {\"claimed_cannot_do\":false,\"evidence\":\"\"}.",
					"Set claimed_cannot_do true when the output says or implies it lacks real-time access, internet access, browsing ability, current-time ability, or tells the user to check elsewhere because it cannot answer.",
					"Set claimed_cannot_do false for normal factual answers, command-derived answers, uncertainty about facts, or absence of capability refusal.",
					"evidence must quote or summarize the shortest phrase that triggered the decision.",
				}, " "),
			},
			{Role: "user", Content: `{"model_output":` + quoteJSONForTest(modelOutput) + `}`},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"claimed_cannot_do": map[string]interface{}{"type": "boolean"},
				"evidence":          map[string]interface{}{"type": "string"},
			},
			"required": []string{"claimed_cannot_do", "evidence"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 128,
		},
	})
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during blind capability review: %v", err)
		}
		t.Fatalf("blind capability review failed: %v", err)
	}
	var review blindCapabilityReview
	if err := json.Unmarshal([]byte(resp.Content), &review); err != nil {
		t.Fatalf("decode blind capability review: %v\n%s", err, resp.Content)
	}
	return review
}

func virginiaTimeEvidenceMatches(evidence string, baselineHour int) bool {
	return clockEvidenceMatchesHour(evidence, baselineHour)
}

func pattayaTimeEvidenceMatches(evidence string, baselineHour int) bool {
	return clockEvidenceMatchesHour(evidence, baselineHour) ||
		strings.Contains(strings.ToLower(evidence), "ict") ||
		strings.Contains(strings.ToLower(evidence), "asia/bangkok")
}

func clockEvidenceMatchesHour(evidence string, baselineHour int) bool {
	for _, hour := range []int{baselineHour, (baselineHour + 23) % 24, (baselineHour + 1) % 24} {
		if strings.Contains(evidence, twoDigitHour(hour)+":") {
			return true
		}
		if strings.Contains(evidence, strconv.Itoa(hour)+":") {
			return true
		}
		if strings.Contains(evidence, twelveHourClockPrefix(hour)) {
			return true
		}
	}
	return false
}

func containsFalseRealtimeLimitation(evidence string) bool {
	lower := strings.ToLower(evidence)
	for _, phrase := range []string{
		"as an ai",
		"i am unable",
		"i'm unable",
		"i cannot",
		"i can't",
		"i do not have access",
		"i don't have access",
		"do not have access to real-time",
		"don't have access to real-time",
		"cannot access real-time",
		"can't access real-time",
		"no access to real-time",
		"do not have internet access",
		"don't have internet access",
		"no internet access",
		"cannot browse",
		"can't browse",
		"unable to browse",
		"not able to browse",
		"check the current time",
		"time zone converter",
		"time zone app",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func assertNoFalseCapabilityLimitation(t *testing.T, client *OllamaClient, result CommandDecisionResult, stdout, stderr string) {
	t.Helper()
	evidence := strings.Join([]string{stdout, stderr, result.Answer}, "\n")
	if containsFalseRealtimeLimitation(evidence) {
		t.Fatalf("live ODN output contains false capability limitation\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			result.Command, result.Answer, stdout, stderr, result.Observations)
	}
	if client == nil || strings.TrimSpace(result.Answer) == "" {
		return
	}
	review := reviewModelOutputForBlindCannotDoClaim(t, client, result.Answer)
	if review.ClaimedCannotDo {
		t.Fatalf("blind LLM reviewer found a false cannot-do claim without seeing the prompt\nreview=%#v\nanswer=%q\ncommand=%q\nstdout=%s\nstderr=%s",
			review, result.Answer, result.Command, stdout, stderr)
	}
}

func countStructuredEvents(events []StructuredCommandEvent, eventType string) int {
	count := 0
	for _, evt := range events {
		if evt.Type == eventType {
			count++
		}
	}
	return count
}

func twoDigitHour(hour int) string {
	if hour < 10 {
		return "0" + strconv.Itoa(hour)
	}
	return strconv.Itoa(hour)
}

func twelveHourClockPrefix(hour int) string {
	value := hour % 12
	if value == 0 {
		value = 12
	}
	return twoDigitHour(value) + ":"
}
