package main

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestBuildCodingRunRequestUsesDirectPipelineAndCurrentWorkspace(t *testing.T) {
	request, err := buildCodingRunRequest(
		"  build the complete notes app  ",
		"/work/notes",
		"session-7",
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Instruction != "  build the complete notes app  " {
		t.Fatalf("instruction=%q", request.Instruction)
	}
	if request.Pipeline != model.PipelineCoding {
		t.Fatalf("pipeline=%q want %q", request.Pipeline, model.PipelineCoding)
	}
	if request.Workspace != "/work/notes" {
		t.Fatalf("workspace=%q", request.Workspace)
	}
	if request.SessionID != "session-7" {
		t.Fatalf("session=%q", request.SessionID)
	}
}

func TestBuildCodingRunRequestFailsWithoutInstructionOrWorkspace(t *testing.T) {
	if _, err := buildCodingRunRequest("", "/work/notes", ""); err == nil {
		t.Fatal("empty instruction was accepted")
	}
	if _, err := buildCodingRunRequest("build it", "", ""); err == nil {
		t.Fatal("empty workspace was accepted")
	}
	if _, err := buildCodingRunRequest("build it", "/work/notes", " padded "); err == nil {
		t.Fatal("noncanonical session identifier was accepted")
	}
}
