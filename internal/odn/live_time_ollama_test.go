package odn

import (
	"bytes"
	"context"
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

func virginiaTimeEvidenceMatches(evidence string, baselineHour int) bool {
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
