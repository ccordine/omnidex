package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBoundedCommandPreviewPreservesFrozenArgvAndEncodedFrameBoundary(t *testing.T) {
	argv := []string{"fixture"}
	for range 128 {
		argv = append(argv, strings.Repeat("\x01", 4096))
	}
	full := AgentActivity{Schema: 1, Source: AgentActivityVerification, Kind: AgentActivityCommand, Status: AgentActivityStarted, CheckID: "check", Role: "review", TaskID: "task", CWD: "/project", Argv: argv}
	bounded := BoundAgentActivityCommand(full)
	if err := bounded.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(bounded)
	if !bounded.Truncated || len(encoded) > 48*1024 || len(full.Argv) != 129 || full.Argv[1] != strings.Repeat("\x01", 4096) {
		t.Fatal("display bounds changed frozen argv or exceeded the actual encoded frame budget")
	}
}

func TestVerificationActivityCannotInventNativeIdentities(t *testing.T) {
	activity := AgentActivity{Schema: 1, Source: AgentActivityVerification, Kind: AgentActivityCommandOutput, Status: AgentActivityDelta, CheckID: "check", Role: "review", TaskID: "task", Text: "observed output"}
	if err := activity.Validate(); err != nil {
		t.Fatal(err)
	}
	activity.ThreadID = "invented-thread"
	if err := activity.Validate(); err == nil {
		t.Fatal("deterministic check activity acquired a fabricated native thread")
	}
}
