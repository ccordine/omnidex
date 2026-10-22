package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestRealtimePlanningActivityPreservesProjectionAndBinding(t *testing.T) {
	const source = `{"jobID":17,"channelID":"planning-channel","stepID":29,"attempt":3,"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"candidate_resolved","subject":"Preserve the requested output format.","outcome":"retained"}}`
	var event JobRuntimeEvent
	if err := json.Unmarshal([]byte(source), &event); err != nil {
		t.Fatal(err)
	}
	server := &Server{realtimeHub: NewRealtimeHub()}
	topic, err := realtimeChannelTopic(event.ChannelID)
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := server.realtimeHub.Subscribe([]string{topic}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Unsubscribe()
	if err := server.PublishJobRuntimeEvent(event); err != nil {
		t.Fatal(err)
	}
	var received struct {
		JobID            int64           `json:"jobID"`
		ChannelID        string          `json:"channelID"`
		StepID           int64           `json:"stepID"`
		Attempt          int64           `json:"attempt"`
		RuntimeEvent     string          `json:"runtimeEvent"`
		PlanningActivity json.RawMessage `json:"planningActivity"`
	}
	if err := json.Unmarshal(<-subscription.Messages, &received); err != nil {
		t.Fatal(err)
	}
	var expected, actual map[string]any
	if err := json.Unmarshal([]byte(source), &expected); err != nil {
		t.Fatal(err)
	}
	if len(received.PlanningActivity) == 0 {
		t.Fatal("planning activity disappeared between the worker transport boundary and channel publication")
	}
	if err := json.Unmarshal(received.PlanningActivity, &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected["planningActivity"]) || received.JobID != 17 || received.ChannelID != "planning-channel" || received.StepID != 29 || received.Attempt != 3 || received.RuntimeEvent != "planning_activity" {
		t.Fatalf("planning projection or exact binding changed: %+v", received)
	}
}

func TestRealtimePlanningActivityRejectsMissingProjection(t *testing.T) {
	server := &Server{realtimeHub: NewRealtimeHub()}
	event := JobRuntimeEvent{JobID: 17, ChannelID: "planning-channel", StepID: 29, Attempt: 3, RuntimeEvent: "planning_activity"}
	if err := server.PublishJobRuntimeEvent(event); err == nil || server.realtimeHub.nextMessageID != 0 {
		t.Fatalf("planning event without observed planning facts was published: %+v", event)
	}
}

func TestRealtimePlanningActivityRejectsMisplacedAndInvalidProjection(t *testing.T) {
	for name, fields := range map[string]string{
		"another runtime event":     `"runtimeEvent":"step_start","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"unrelated detail":          `"runtimeEvent":"planning_activity","detail":"private raw output","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"file operation":            `"runtimeEvent":"planning_activity","fileOperation":"create","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"file path":                 `"runtimeEvent":"planning_activity","filePath":"source.go","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"file source path":          `"runtimeEvent":"planning_activity","fileSourcePath":"old.go","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"native activity":           `"runtimeEvent":"planning_activity","agentActivity":{"schema":1,"source":"native","kind":"message","status":"delta","threadID":"thread","turnID":"turn","itemID":"item","role":"plan","text":"Public text"},"planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"call activity":             `"runtimeEvent":"planning_activity","agentCallActivity":{"schema":1,"callID":9,"role":"plan","status":"started"},"planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"unsupported schema":        `"runtimeEvent":"planning_activity","planningActivity":{"schema":2,"kind":"selected","count":1}`,
		"invented kind":             `"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"thinking","subject":"Invented thought"}`,
		"missing candidate":         `"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"candidate_started"}`,
		"unregistered outcome":      `"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"candidate_resolved","subject":"Requested behavior.","outcome":"approved-by-model"}`,
		"negative count":            `"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"inventory_completed","count":-1}`,
		"unrelated partition count": `"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"candidate_resolved","subject":"Requested behavior.","outcome":"retained","count":2}`,
	} {
		t.Run(name, func(t *testing.T) {
			var event JobRuntimeEvent
			if err := json.Unmarshal([]byte(`{"jobID":17,"channelID":"planning-channel","stepID":29,"attempt":3,`+fields+`}`), &event); err != nil {
				t.Fatal(err)
			}
			server := &Server{realtimeHub: NewRealtimeHub()}
			if err := server.PublishJobRuntimeEvent(event); err == nil || server.realtimeHub.nextMessageID != 0 {
				t.Fatalf("misplaced or invalid planning fact was published: %+v", event)
			}
		})
	}
}

func TestRealtimePlanningActivityPublishesObservedCountsAndDispositions(t *testing.T) {
	activities := []model.PlanningActivity{
		{Schema: 1, Kind: "inventory_started"},
		{Schema: 1, Kind: "inventory_completed", Count: 3},
		{Schema: 1, Kind: "candidate_started", Subject: "Preserve the requested output format."},
		{Schema: 1, Kind: "candidate_resolved", Subject: "Preserve the requested output format.", Outcome: "partitioned", Count: 2},
		{Schema: 1, Kind: "selected", Count: 2},
	}
	for _, outcome := range []string{"retained", "excluded", "unrequested", "duplicate", "unresolved", "deferred"} {
		activities = append(activities, model.PlanningActivity{Schema: 1, Kind: "candidate_resolved", Subject: "Preserve the requested output format.", Outcome: outcome})
	}
	server := &Server{realtimeHub: NewRealtimeHub()}
	topic, err := realtimeChannelTopic("planning-channel")
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := server.realtimeHub.Subscribe([]string{topic}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Unsubscribe()
	for _, activity := range activities {
		event := JobRuntimeEvent{JobID: 17, ChannelID: "planning-channel", StepID: 29, Attempt: 3, RuntimeEvent: "planning_activity", PlanningActivity: &activity}
		if err := server.PublishJobRuntimeEvent(event); err != nil {
			t.Fatalf("publish observed %s/%s: %v", activity.Kind, activity.Outcome, err)
		}
		var received client.RealtimeEvent
		if err := json.Unmarshal(<-subscription.Messages, &received); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(received.PlanningActivity, &activity) || received.ChannelID != event.ChannelID || received.JobID != event.JobID || received.StepID != event.StepID || received.Attempt != event.Attempt {
			t.Fatalf("observed counts, disposition, or binding changed: %+v", received)
		}
	}
}

func TestRealtimePlanningActivityPreservesSemanticObservation(t *testing.T) {
	const projection = `{"schema":1,"kind":"semantic_completed","station":"application_requirement_candidate_authorization","workKind":"application_requirement_candidate_authorization","model":"gpt-6.1-sol","callID":17,"question":"Is the complete semantic content of this exact unclassified candidate entailed by the immutable current request?","why":"Semantic entailment, including direct-imperative grammatical normalization, purpose-name core meaning, and construction-only constraints, cannot be established by syntax or byte comparison.","consumer":"DecodeApplicationRequirementCandidateAuthorizationResult validates the relation; code discards a not-entailed candidate before classification and continues independent candidates without reopening accepted requirements.","options":["A. Required by request","B. Not required"],"subject":"Allow pending requests to be canceled.","against":"A pending request can be canceled.","result":"A","truncated":true}`
	for name, projection := range map[string]string{
		"model completed": projection,
		"model started":   strings.Replace(strings.Replace(projection, `"kind":"semantic_completed"`, `"kind":"semantic_started"`, 1), `,"result":"A","truncated":true`, "", 1),
		"code resolved":   strings.Replace(strings.Replace(projection, `"model":"gpt-6.1-sol",`, "", 1), `"result":"A","truncated":true`, `"result":"A","inferenceFree":true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			var event JobRuntimeEvent
			if err := json.Unmarshal([]byte(`{"jobID":17,"channelID":"planning-channel","stepID":29,"attempt":3,"runtimeEvent":"planning_activity","planningActivity":`+projection+`}`), &event); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(event.PlanningActivity)
			if err != nil {
				t.Fatal(err)
			}
			var expected, decoded map[string]any
			if err := json.Unmarshal([]byte(projection), &expected); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded, expected) {
				t.Fatalf("semantic call observations were discarded at the API transport boundary: got %+v, want %+v", decoded, expected)
			}
			server := &Server{realtimeHub: NewRealtimeHub()}
			subscription, err := server.realtimeHub.Subscribe([]string{"channel:planning-channel"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer subscription.Unsubscribe()
			if err := server.PublishJobRuntimeEvent(event); err != nil {
				t.Fatal(err)
			}
			var message map[string]any
			if err := json.Unmarshal(<-subscription.Messages, &message); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(message["planningActivity"], expected) {
				t.Fatalf("semantic call observations changed during channel publication: %+v", message)
			}
		})
	}
}
