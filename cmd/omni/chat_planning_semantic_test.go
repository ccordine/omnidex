package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/client"
)

func TestPlanningNormalShowsEachExactSemanticQuestionOptionsAndResult(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"check>", semanticPlanningQuestion, semanticPlanningSubject, semanticPlanningAgainst,
		semanticPlanningWhy, semanticPlanningOptionA, semanticPlanningOptionB,
	} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("check omitted exact fact %q: %q", required, output.String())
		}
	}
	if strings.Contains(output.String(), "Result:") || strings.Contains(output.String(), "completed") {
		t.Fatalf("a started check claimed a returned result: %q", output.String())
	}
	output.Reset()
	if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_completed", semanticPlanningResult)); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"check>", semanticPlanningSubject, semanticPlanningResult, semanticPlanningConsumer} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("returned check omitted result/action %q: %q", required, output.String())
		}
	}
	for _, forbidden := range []string{"I think", "I decided", "thinking>", "callID", "semantic_completed"} {
		if strings.Contains(output.String(), forbidden) {
			t.Errorf("code-derived observation appeared as invented thought/diagnostic %q: %q", forbidden, output.String())
		}
	}
}

func TestPlanningSpinnerKeepsExactCandidateDuringSemanticCheck(t *testing.T) {
	for _, verbosity := range []chatVerbosity{chatNormal, chatVerbose, chatDebug} {
		t.Run(verbosity.String(), func(t *testing.T) {
			console, _ := planReviewTestConsole(t)
			if err := console.SetActivity(chatActivity{JobID: 41, StepID: 12, Label: "Planning", Working: true}); err != nil {
				t.Fatal(err)
			}
			session := chatSession{renderer: chatRenderer{console: console, verbosity: verbosity}}
			if err := session.acceptRealtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{semanticPlanningSubject, semanticPlanningQuestion} {
				if !strings.Contains(console.activityDetail, required) {
					t.Errorf("live semantic check omitted %q: %q", required, console.activityDetail)
				}
			}
		})
	}
}

func TestPlanningCheckIdentifiesItsModelCallAndCodeRule(t *testing.T) {
	for _, verbosity := range []chatVerbosity{chatNormal, chatVerbose, chatDebug} {
		t.Run(verbosity.String(), func(t *testing.T) {
			console, output := planReviewTestConsole(t)
			renderer := chatRenderer{console: console, verbosity: verbosity}
			if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_completed", semanticPlanningResult)); err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{"fixture-model", "Call: 17", "Code rule: " + semanticPlanningConsumer} {
				if !strings.Contains(output.String(), required) {
					t.Errorf("check omitted its exact identity/consumer meaning %q: %q", required, output.String())
				}
			}
			if strings.Contains(output.String(), "Action: "+semanticPlanningConsumer) {
				t.Fatalf("consumer alternatives were presented as an actual completed action: %q", output.String())
			}
		})
	}
}

func TestPlanningCodeResolvedResultDoesNotClaimAModelCall(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	event := semanticPlanningJSONEvent(t, "semantic_completed", semanticPlanningResult)
	event.PlanningActivity.InferenceFree = true
	event.PlanningActivity.CallID = 0
	if err := renderer.realtime(event); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Resolved by code.") || strings.Contains(output.String(), "Model: fixture-model") || strings.Contains(output.String(), "Call:") {
		t.Fatalf("code-resolved result claimed an inference or hid its real source: %q", output.String())
	}
}

func TestPlanningStartedCheckDoesNotInventAJournalCallID(t *testing.T) {
	for _, verbosity := range []chatVerbosity{chatNormal, chatVerbose, chatDebug} {
		t.Run(verbosity.String(), func(t *testing.T) {
			console, output := planReviewTestConsole(t)
			renderer := chatRenderer{console: console, verbosity: verbosity}
			if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "Call:") {
				t.Fatalf("started check invented an unreserved journal identity: %q", output.String())
			}
		})
	}
}

const (
	semanticPlanningStation  = "application_requirement_candidate_duplicate"
	semanticPlanningSubject  = "Preserve saved draft content"
	semanticPlanningAgainst  = "Store drafts locally"
	semanticPlanningQuestion = "Does this candidate duplicate the retained outcome?"
	semanticPlanningWhy      = "Byte equality does not determine semantic duplication."
	semanticPlanningOptionA  = "A. The candidate duplicates the retained outcome."
	semanticPlanningOptionB  = "B. The candidate adds a distinct outcome."
	semanticPlanningResult   = "B. The candidate adds a distinct outcome."
	semanticPlanningConsumer = "Code maps the selected relation to the existing candidate disposition."
)

func semanticPlanningJSONEvent(t *testing.T, kind, result string) client.RealtimeEvent {
	t.Helper()
	callID := 0
	if kind == "semantic_completed" {
		callID = 17
	}
	payload, err := json.Marshal(map[string]any{
		"schema": 1, "kind": kind, "station": semanticPlanningStation, "workKind": "application_requirement_candidate_duplicate",
		"model": "fixture-model", "callID": callID,
		"subject": semanticPlanningSubject, "against": semanticPlanningAgainst,
		"question": semanticPlanningQuestion, "why": semanticPlanningWhy,
		"options":  []string{semanticPlanningOptionA, semanticPlanningOptionB},
		"consumer": semanticPlanningConsumer, "result": result,
	})
	if err != nil {
		t.Fatal(err)
	}
	return planningJSONEvent(t, string(payload))
}
