package codingagent

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPlanningMixedParagraphKeepsPolicyWithoutSessionWarning(t *testing.T) {
	for _, fixture := range []struct{ name, policy, warning string }{
		{"archive", "Keep archive files local.", "This planning session is read-only."},
		{"currency", "Use only the standard library.", "This conversation cannot approve commands."},
		{"lasting read-only policy", "The deployed filesystem must be read-only.", "The current planning tool has no write access."},
	} {
		for _, warningFirst := range []bool{false, true} {
			t.Run(fixture.name+"/"+map[bool]string{false: "policy first", true: "warning first"}[warningFirst], func(t *testing.T) {
				source := fixture.policy + " " + fixture.warning
				if warningFirst {
					source = fixture.warning + " " + fixture.policy
				}
				call := Assignment{JobID: 1, Role: PlanningInitial, Route: Route{Provider: "fixture", Model: "fixture"}, PlanningResponse: source}
				projections := 0
				provider := sieveTestProvider(t, func(leaf Assignment) any {
					step := leaf.ParserStep
					if step.Stage == "scan" {
						scan := parserScanResult{Units: map[string]parserUnitDisposition{}}
						for _, unit := range parserSourceUnits(step.Source) {
							kind := ""
							switch strings.TrimSpace(unit.Text) {
							case fixture.policy:
								kind = "project_constraint"
							case fixture.warning:
								kind = "ignore"
							default:
								t.Fatalf("policy and session observation cannot be independently classified: %q", unit.Text)
							}
							scan.Units[unit.ID] = parserUnitDisposition{Kind: kind, Reason: "fixed scope oracle", GroupStart: unit.ID}
						}
						return scan
					}
					projections++
					if strings.TrimSpace(step.Source) != fixture.policy {
						t.Fatalf("session-only text reached a ledger mutation: %q", step.Source)
					}
					return map[string]any{"operations": []any{map[string]any{"kind": "add", "rationale": "stated lasting policy",
						"item": map[string]any{"constraint": map[string]any{"id": step.NewIDs["constraint"], "priority": "normal", "rule": "code retains the exact source"}}}}}
				})
				result, err := RunParserSieve(t.Context(), call, func(NativeEvent) error { return nil }, provider)
				if err != nil {
					t.Fatal(err)
				}
				var parsed struct{ Ledger EngineeringLedger }
				if err := json.Unmarshal(result.Content, &parsed); err != nil {
					t.Fatal(err)
				}
				if projections != 1 || len(parsed.Ledger.Constraints) != 1 || parsed.Ledger.Constraints[0].Rule != fixture.policy || strings.Contains(string(result.Content), fixture.warning) {
					t.Fatalf("mixed paragraph corrupted the proposed policy: %s", result.Content)
				}
			})
		}
	}
}

func TestSentenceUnitsPreserveBytesCodePathsAndListMarkers(t *testing.T) {
	for _, fixture := range []struct {
		source string
		count  int
	}{
		{"1. Run `tool --text 'one. two'` for https://docs.example/app.js. Preserve café.txt and version 1.2.3.\n", 2},
		{"First clause.\tSecond clause! Third clause?\n\n", 4},
		{"```text\nkeep. this unchanged.\n```\n", 3},
		{"~~~text\nkeep. this unchanged.\n~~~\n", 3},
	} {
		units := parserSourceUnits(fixture.source)
		var recovered strings.Builder
		for _, unit := range units {
			if !utf8.ValidString(unit.Text) {
				t.Fatal("unit split a UTF-8 character")
			}
			recovered.WriteString(unit.Text)
		}
		if recovered.String() != fixture.source || len(units) != fixture.count {
			t.Fatalf("source bytes or syntax boundaries changed: %#v", units)
		}
	}
}
