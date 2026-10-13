package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/model"
)

func TestRealtimePlanningActivityPreservesProjectionAndBinding(t *testing.T) {
	const source = `{"id":5,"eventName":"job-runtime-event","jobID":17,"channelID":"planning-channel","stepID":29,"attempt":3,"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"candidate_resolved","subject":"Preserve the requested output format.","outcome":"retained"}}`
	var event RealtimeEvent
	if err := json.Unmarshal([]byte(source), &event); err != nil {
		t.Fatal(err)
	}
	if err := event.validate("planning-channel"); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var expected, received map[string]any
	if err := json.Unmarshal([]byte(source), &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &received); err != nil {
		t.Fatal(err)
	}
	if received["planningActivity"] == nil {
		t.Fatal("planning activity disappeared while decoding the channel event")
	}
	if !reflect.DeepEqual(received["planningActivity"], expected["planningActivity"]) || event.ID != 5 || event.JobID != 17 || event.ChannelID != "planning-channel" || event.StepID != 29 || event.Attempt != 3 || event.RuntimeEvent != "planning_activity" {
		t.Fatalf("planning projection or exact binding changed: %+v", event)
	}
}

func TestRealtimePlanningActivityRejectsMissingProjection(t *testing.T) {
	event := RealtimeEvent{ID: 5, EventName: RealtimeJobRuntimeEvent, JobID: 17, ChannelID: "planning-channel", StepID: 29, Attempt: 3, RuntimeEvent: "planning_activity"}
	if err := event.validate("planning-channel"); err == nil {
		t.Fatalf("planning event without observed planning facts was accepted: %+v", event)
	}
}

func TestRealtimePlanningActivityRejectsMisplacedAndInvalidProjection(t *testing.T) {
	for name, fields := range map[string]string{
		"connection frame":      `"eventName":"realtime-connected","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"progress frame":        `"eventName":"job-progress","id":5,"jobID":17,"phase":"state_changed","summary":"Working","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"another runtime event": `"eventName":"job-runtime-event","id":5,"jobID":17,"stepID":29,"attempt":3,"runtimeEvent":"step_start","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"unrelated detail":      `"eventName":"job-runtime-event","id":5,"jobID":17,"stepID":29,"attempt":3,"runtimeEvent":"planning_activity","detail":"private raw output","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"native event":          `"eventName":"job-runtime-event","id":5,"jobID":17,"stepID":29,"attempt":3,"runtimeEvent":"agent_activity","agentActivity":{"schema":1,"source":"native","kind":"message","status":"delta","threadID":"thread","turnID":"turn","itemID":"item","role":"plan","text":"Public text"},"planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"call event":            `"eventName":"job-runtime-event","id":5,"jobID":17,"stepID":29,"attempt":3,"runtimeEvent":"agent_call_activity","agentCallActivity":{"schema":1,"callID":9,"role":"plan","status":"started"},"planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"file event":            `"eventName":"job-runtime-event","id":5,"jobID":17,"stepID":29,"attempt":3,"runtimeEvent":"workspace_file_changed","fileOperation":"create","filePath":"source.go","planningActivity":{"schema":1,"kind":"selected","count":1}`,
		"invalid schema":        `"eventName":"job-runtime-event","id":5,"jobID":17,"stepID":29,"attempt":3,"runtimeEvent":"planning_activity","planningActivity":{"schema":2,"kind":"selected","count":1}`,
		"invented kind":         `"eventName":"job-runtime-event","id":5,"jobID":17,"stepID":29,"attempt":3,"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"thinking","subject":"Invented thought"}`,
		"negative count":        `"eventName":"job-runtime-event","id":5,"jobID":17,"stepID":29,"attempt":3,"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"selected","count":-1}`,
	} {
		t.Run(name, func(t *testing.T) {
			var event RealtimeEvent
			if err := json.Unmarshal([]byte(`{"channelID":"planning-channel",`+fields+`}`), &event); err != nil {
				t.Fatal(err)
			}
			if err := event.validate("planning-channel"); err == nil {
				t.Fatalf("misplaced or invalid planning fact was accepted: %+v", event)
			}
		})
	}
}

func TestRealtimePlanningActivityDecodesLiveStream(t *testing.T) {
	const source = `{"id":5,"eventName":"job-runtime-event","jobID":17,"channelID":"planning-channel","stepID":29,"attempt":3,"runtimeEvent":"planning_activity","planningActivity":{"schema":1,"kind":"candidate_resolved","subject":"Preserve the requested output format.","outcome":"partitioned","count":2}}`
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requireRequestAuthority(t, request, http.MethodGet, "/v1/realtime/ws", "channel_id=planning-channel&topics=jobs&workspace_identity="+testWorkspaceIdentity)
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer connection.Close()
		if err := connection.WriteMessage(websocket.TextMessage, []byte(source)); err != nil {
			t.Errorf("write planning event: %v", err)
		}
	}))
	defer server.Close()
	stream, err := testClient(t, server.URL).OpenJobEvents(context.Background(), "planning-channel", testWorkspaceIdentity, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	event, err := stream.Read()
	if err != nil {
		t.Fatal(err)
	}
	expected := &model.PlanningActivity{Schema: 1, Kind: "candidate_resolved", Subject: "Preserve the requested output format.", Outcome: "partitioned", Count: 2}
	if !reflect.DeepEqual(event.PlanningActivity, expected) || event.ID != 5 || event.JobID != 17 || event.ChannelID != "planning-channel" || event.StepID != 29 || event.Attempt != 3 {
		t.Fatalf("live planning projection or exact binding changed: %+v", event)
	}
}

func TestRealtimePlanningActivityRequiresExactChannelAndStep(t *testing.T) {
	valid := RealtimeEvent{ID: 5, EventName: RealtimeJobRuntimeEvent, JobID: 17, ChannelID: "planning-channel", StepID: 29, Attempt: 3, RuntimeEvent: "planning_activity", PlanningActivity: &model.PlanningActivity{Schema: 1, Kind: "selected", Count: 2}}
	for name, mutate := range map[string]func(*RealtimeEvent){
		"foreign channel":        func(event *RealtimeEvent) { event.ChannelID = "foreign-channel" },
		"missing event identity": func(event *RealtimeEvent) { event.ID = 0 },
		"missing job":            func(event *RealtimeEvent) { event.JobID = 0 },
		"missing step":           func(event *RealtimeEvent) { event.StepID = 0 },
		"missing attempt":        func(event *RealtimeEvent) { event.Attempt = 0 },
		"file operation":         func(event *RealtimeEvent) { event.FileOperation = "create" },
		"file path":              func(event *RealtimeEvent) { event.FilePath = "source.go" },
		"file source path":       func(event *RealtimeEvent) { event.FileSourcePath = "old.go" },
		"native activity":        func(event *RealtimeEvent) { event.AgentActivity = &model.AgentActivity{} },
		"call activity":          func(event *RealtimeEvent) { event.AgentCallActivity = &model.AgentCallActivity{} },
	} {
		t.Run(name, func(t *testing.T) {
			event := valid
			mutate(&event)
			if err := event.validate("planning-channel"); err == nil {
				t.Fatalf("planning fact lost its exact authority boundary: %+v", event)
			}
		})
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
			var event RealtimeEvent
			if err := json.Unmarshal([]byte(`{"id":5,"eventName":"job-runtime-event","jobID":17,"channelID":"planning-channel","stepID":29,"attempt":3,"runtimeEvent":"planning_activity","planningActivity":`+projection+`}`), &event); err != nil {
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
				t.Fatalf("semantic call observations were discarded during channel decode: got %+v, want %+v", decoded, expected)
			}
			if err := event.validate("planning-channel"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
