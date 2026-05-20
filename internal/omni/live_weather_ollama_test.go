package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type liveWeatherSnapshot struct {
	Location    string
	TempC       string
	Humidity    string
	Description string
	Raw         string
}

func TestLiveOllamaFindsThailandWeatherFromWebEvidence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live weather scrape in short mode")
	}
	skipUnlessLiveOllamaEnabled(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute
	baseline := scrapeLiveThailandWeather(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecision(ctx, "What is the current weather in Bangkok, Thailand?", client, stdout, stderr)
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during live weather test: %v", err)
		}
		t.Fatal(err)
	}
	if len(result.Observations) == 0 {
		t.Fatal("expected at least one command observation")
	}
	if !hasRealCommandObservation(result.Observations) {
		t.Fatalf("expected real command observation: %#v", result.Observations)
	}

	evidence := strings.Join([]string{stdout.String(), stderr.String(), result.Answer}, "\n")
	assertNoFalseCapabilityLimitation(t, client, result, stdout.String(), stderr.String())
	if !liveWeatherEvidenceMatchesBaseline(evidence, baseline) {
		t.Fatalf(
			"Omni weather evidence did not overlap baseline\nbaseline: location=%s temp_C=%s humidity=%s desc=%s\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s",
			baseline.Location,
			baseline.TempC,
			baseline.Humidity,
			baseline.Description,
			result.Command,
			result.Answer,
			stdout.String(),
			stderr.String(),
		)
	}
}

func scrapeLiveThailandWeather(t *testing.T) liveWeatherSnapshot {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://wttr.in/Bangkok?format=j1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("live weather source unavailable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Skipf("live weather source returned status %d", resp.StatusCode)
	}
	blob, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}

	var decoded struct {
		CurrentCondition []struct {
			TempC       string `json:"temp_C"`
			Humidity    string `json:"humidity"`
			WeatherDesc []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
		} `json:"current_condition"`
	}
	if err := json.Unmarshal(blob, &decoded); err != nil {
		t.Fatalf("parse live weather JSON: %v\n%s", err, string(blob))
	}
	if len(decoded.CurrentCondition) == 0 {
		t.Fatalf("live weather JSON missing current_condition: %s", string(blob))
	}
	condition := decoded.CurrentCondition[0]
	description := ""
	if len(condition.WeatherDesc) > 0 {
		description = condition.WeatherDesc[0].Value
	}
	if strings.TrimSpace(condition.TempC) == "" || strings.TrimSpace(condition.Humidity) == "" || strings.TrimSpace(description) == "" {
		t.Fatalf("live weather JSON missing expected fields: %s", string(blob))
	}
	return liveWeatherSnapshot{
		Location:    "Bangkok, Thailand",
		TempC:       condition.TempC,
		Humidity:    condition.Humidity,
		Description: description,
		Raw:         string(blob),
	}
}

func liveWeatherEvidenceMatchesBaseline(evidence string, baseline liveWeatherSnapshot) bool {
	lowerEvidence := strings.ToLower(evidence)
	description := strings.ToLower(baseline.Description)
	return strings.Contains(lowerEvidence, strings.ToLower(baseline.Location)) ||
		strings.Contains(lowerEvidence, fmt.Sprintf(`"temp_C":"%s"`, baseline.TempC)) ||
		strings.Contains(lowerEvidence, fmt.Sprintf(`"humidity":"%s"`, baseline.Humidity)) ||
		strings.Contains(lowerEvidence, description)
}
