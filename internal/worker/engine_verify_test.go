package worker

import (
	"math"
	"strings"
	"testing"
)

func TestDecodeVerificationOutcomeAcceptsStringGaps(t *testing.T) {
	payload := `{"status":"pass","confidence":0.87,"summary":"ok","gaps":"missing evidence","cannot_complete_reason":""}`
	outcome, err := decodeVerificationOutcome(payload)
	if err != nil {
		t.Fatalf("decodeVerificationOutcome returned error: %v", err)
	}
	if outcome.Status != "pass" {
		t.Fatalf("status=%q, want pass", outcome.Status)
	}
	if len(outcome.Gaps) != 1 || outcome.Gaps[0] != "missing evidence" {
		t.Fatalf("gaps=%v, want [missing evidence]", outcome.Gaps)
	}
}

func TestDecodeVerificationOutcomeAcceptsArrayGapsAndStringConfidence(t *testing.T) {
	payload := `{"status":"retry","confidence":"0.42","summary":"needs changes","gaps":["gap one","gap two"],"cannot_complete_reason":""}`
	outcome, err := decodeVerificationOutcome(payload)
	if err != nil {
		t.Fatalf("decodeVerificationOutcome returned error: %v", err)
	}
	if outcome.Confidence != 0.42 {
		t.Fatalf("confidence=%v, want 0.42", outcome.Confidence)
	}
	if len(outcome.Gaps) != 2 {
		t.Fatalf("gaps=%v, want 2 entries", outcome.Gaps)
	}
}

func TestApplyVerificationEvaluatorFallbackPolicyDowngradesPass(t *testing.T) {
	initial := verificationOutcome{
		Status:     "pass",
		Confidence: 0.91,
		Summary:    "verification completed",
	}
	adjusted := applyVerificationEvaluatorFallbackPolicy(initial, 2)
	if adjusted.Status != "retry" {
		t.Fatalf("status=%q, want retry", adjusted.Status)
	}
	if adjusted.Confidence > 0.25 {
		t.Fatalf("confidence=%v, want <=0.25", adjusted.Confidence)
	}
	if len(adjusted.Gaps) == 0 {
		t.Fatal("expected fallback policy to add a gap note")
	}
}

func TestDecodeVerificationOutcomeParsesNonJSONStatusMarkers(t *testing.T) {
	payload := "INCOMPLETE: create index.html first\nNext action required: `touch index.html`"
	outcome, err := decodeVerificationOutcome(payload)
	if err != nil {
		t.Fatalf("decodeVerificationOutcome returned error: %v", err)
	}
	if outcome.Status != "retry" {
		t.Fatalf("status=%q, want retry", outcome.Status)
	}
	if !strings.Contains(strings.ToLower(outcome.Summary), "create index.html") {
		t.Fatalf("summary=%q, want create index.html hint", outcome.Summary)
	}
}

func TestParseVerificationConfidenceFieldSupportsLabelsAndPercentages(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    float64
		wantOk  bool
		wantErr bool
	}{
		{name: "numeric fraction", raw: `0.42`, want: 0.42, wantOk: true},
		{name: "numeric percent scale", raw: `90`, want: 0.90, wantOk: true},
		{name: "string percent", raw: `"90%"`, want: 0.90, wantOk: true},
		{name: "string label", raw: `"high"`, want: 0.80, wantOk: true},
		{name: "empty string", raw: `""`, wantOk: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := parseVerificationConfidenceField([]byte(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for raw=%s", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for raw=%s: %v", tc.raw, err)
			}
			if ok != tc.wantOk {
				t.Fatalf("ok=%v want=%v for raw=%s", ok, tc.wantOk, tc.raw)
			}
			if !ok {
				return
			}
			if math.Abs(got-tc.want) > 0.001 {
				t.Fatalf("confidence=%v want=%v for raw=%s", got, tc.want, tc.raw)
			}
		})
	}
}

func TestEvaluateDeterministicLocalActionReviewComplete(t *testing.T) {
	instruction := strings.Join([]string{
		"Deterministic post-action review step (required):",
		"",
		"Original user request:",
		"in the current directory, let's make a index.html file",
		"",
		"Local capability kind:",
		"local_shell",
		"",
		"Executed local action output:",
		"Executed: touch index.html",
		"Created file: /tmp/index.html",
		"Executed: ls -l index.html",
	}, "\n")

	outcome, response, ok := evaluateDeterministicLocalActionReview(instruction)
	if !ok {
		t.Fatal("expected deterministic local action review to match")
	}
	if outcome.Status != "pass" {
		t.Fatalf("status=%q want pass", outcome.Status)
	}
	if !strings.HasPrefix(response, "COMPLETE:") {
		t.Fatalf("response=%q want COMPLETE prefix", response)
	}
	if !strings.Contains(response, "ls -l") {
		t.Fatalf("response=%q want verification command", response)
	}
}

func TestEvaluateDeterministicLocalActionReviewDetectsTargetMismatch(t *testing.T) {
	instruction := strings.Join([]string{
		"Deterministic post-action review step (required):",
		"",
		"Original user request:",
		"create file `index.html` in this directory",
		"",
		"Local capability kind:",
		"local_shell",
		"",
		"Executed local action output:",
		"Executed: touch test",
		"Created file: /tmp/test",
	}, "\n")

	outcome, response, ok := evaluateDeterministicLocalActionReview(instruction)
	if !ok {
		t.Fatal("expected deterministic local action review to match")
	}
	if outcome.Status != "retry" {
		t.Fatalf("status=%q want retry", outcome.Status)
	}
	if !strings.HasPrefix(response, "INCOMPLETE:") {
		t.Fatalf("response=%q want INCOMPLETE prefix", response)
	}
	if !strings.Contains(response, "touch \"index.html\"") {
		t.Fatalf("response=%q want next-action touch index.html", response)
	}
}

func TestParseTournamentConfidenceSupportsTextLabels(t *testing.T) {
	if got := parseTournamentConfidence("CONFIDENCE: High"); got != 80 {
		t.Fatalf("confidence=%d want 80", got)
	}
	if got := parseTournamentConfidence("CONFIDENCE: 92"); got != 92 {
		t.Fatalf("confidence=%d want 92", got)
	}
}
