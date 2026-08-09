package queue

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func TestDecodeTaskEventColumnsRejectsUnknownTrailingAndInvalidShapes(t *testing.T) {
	t.Parallel()
	ledgerID := taskstate.LedgerID("ledger_" + strings.Repeat("a", 64))
	commandID := taskstate.CommandID("command_" + strings.Repeat("b", 64))
	commandSHA := strings.Repeat("c", 64)
	event := taskstate.Event{
		LedgerID: ledgerID, Version: 1, CommandID: commandID,
		CommandSHA256: commandSHA, CommandKind: taskstate.CommandAddNode,
		Kind: taskstate.EventNodeAdded, Authority: taskstate.AuthorityCode,
		Node: &taskstate.Node{
			ID: "node-one", Kind: taskstate.NodeTask, Title: "Node one",
			Status: taskstate.NodePending, Priority: 1, CreatedBy: taskstate.AuthorityCode,
			VerificationRefs: []taskstate.Ref{}, AcceptanceCriteria: []string{},
			Metadata:       taskstate.EmptyJSONObject(),
			CreatedVersion: 1, UpdatedVersion: 1,
		},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	decode := func(raw []byte) error {
		_, err := decodeTaskEventColumns(
			ledgerID, 1, commandID, commandSHA, taskstate.CommandAddNode,
			taskstate.EventNodeAdded, taskstate.AuthorityCode, nil, raw,
		)
		return err
	}
	if err := decode(payload); err != nil {
		t.Fatalf("valid event payload failed: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	fields["step_id"] = nil
	nullStep, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := decode(nullStep); err == nil || !strings.Contains(err.Error(), "step presence") {
		t.Fatalf("null event step error=%v", err)
	}
	delete(fields, "step_id")
	fields["entry_id"] = ""
	forbidden, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := decode(forbidden); err == nil || !strings.Contains(err.Error(), "forbidden field") {
		t.Fatalf("forbidden event field error=%v", err)
	}
	fields["unknown_field"] = true
	unknown, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := decode(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown event field error=%v", err)
	}
	if err := decode(append(payload, []byte(` {}`)...)); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing event JSON error=%v", err)
	}

	missingNode := event
	missingNode.Node = nil
	missingPayload, err := json.Marshal(missingNode)
	if err != nil {
		t.Fatal(err)
	}
	if err := decode(missingPayload); !errors.Is(err, taskstate.ErrInvalidState) {
		t.Fatalf("empty node-added event error=%v", err)
	}
	extraProjection := event
	extraProjection.Edge = &taskstate.Edge{ID: "edge-one"}
	if err := validateTaskEventShape(extraProjection, nil); !errors.Is(err, taskstate.ErrInvalidState) {
		t.Fatalf("extra event projection error=%v", err)
	}
}
