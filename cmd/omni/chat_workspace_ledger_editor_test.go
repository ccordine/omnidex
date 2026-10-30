package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gryph/omnidex/internal/model"
)

func TestLedgerClickOpensNamedItemAndManualEditKeepsOtherDetails(t *testing.T) {
	view, _ := testWorkspace(t, 110, 32)
	openLedgerPane(t, view)
	snapshot := panelLedgerFixture()
	snapshot.Ledger.Tasks[1].OwnerContext = []model.TaskContext{{Text: "Keep this\nmultiline context.", Provenance: snapshot.Ledger.Tasks[1].Provenance}}
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	row := -1
	for _, hit := range view.ledger.hits {
		if hit.item == "panel-second" {
			row = hit.row
		}
	}
	if row < 0 {
		t.Fatal("named item has no click target")
	}
	if err := view.ledgerMouse(fmt.Sprintf("\x1b[<0;4;%dM", row+3)); err != nil {
		t.Fatal(err)
	}
	if view.ledger.selected != "panel-second" || !view.ledger.details {
		t.Fatal("click did not open the selected item")
	}
	if err := view.key('e', false); err != nil {
		t.Fatal(err)
	}
	if err := view.key(21, false); err != nil {
		t.Fatal(err)
	}
	for _, value := range []byte("Renamed 界 item") {
		if err := view.key(value, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, size := range [][2]int{{40, 12}, {110, 32}} {
		if _, _, err := view.Resize(size[0], size[1]); err != nil {
			t.Fatal(err)
		}
		frame, _, _, visible := workspaceFrame(view)
		if !visible {
			t.Fatal("manual editor cursor was lost")
		}
		for _, line := range frame {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("editor overflowed on resize")
			}
		}
	}
	// A background refresh must not consume an explicit Save.
	view.ledger.loading = true
	if err := view.key(19, false); err != nil {
		t.Fatal(err)
	}
	request := <-view.ledger.requests
	if request.action.Kind != model.LedgerMutationAmend || request.action.TargetID != "panel-second" || request.action.Item.Goal != "Renamed 界 item" {
		t.Fatalf("wrong manual edit: %+v", request)
	}
	if !reflect.DeepEqual(request.action.Item.OwnerContext, snapshot.Ledger.Tasks[1].OwnerContext) {
		t.Fatal("editing a name rewrote untouched multiline context")
	}
	if view.ledger.snapshot.Ledger.Tasks[1].Goal != snapshot.Ledger.Tasks[1].Goal {
		t.Fatal("manual save fabricated a server result")
	}
}

func TestLedgerCreationPasteAndCancelStayInsidePane(t *testing.T) {
	view, _ := testWorkspace(t, 100, 24)
	openLedgerPane(t, view)
	if err := view.acceptLedger(panelLedgerFixture()); err != nil {
		t.Fatal(err)
	}
	if err := view.key('n', false); err != nil {
		t.Fatal(err)
	}
	source := &workspaceInputSource{view: view}
	for _, value := range []byte("\x1b[200~New 界 item\x1b[201~") {
		if err := source.consume(value); err != nil {
			t.Fatal(err)
		}
	}
	if view.focus != workspaceLedgerPane || view.ledger.editor.fields[0] != "New 界 item" {
		t.Fatal("paste left the item editor")
	}
	for _, value := range []byte("\x1b[<0;9;8M") {
		if err := source.consume(value); err != nil {
			t.Fatal(err)
		}
	}
	if view.ledger.editor.fields[0] != "New 界 item" {
		t.Fatal("mouse sequence was inserted into the draft")
	}
	if err := view.key(19, false); err != nil {
		t.Fatal(err)
	}
	request := <-view.ledger.requests
	if request.action.Kind != model.LedgerMutationAdd || request.action.Item.Goal != "New 界 item" {
		t.Fatal("creation lost the draft")
	}
	view.ledger.loading = false
	view.ledger.saving = false
	if err := view.key(27, false); err != nil {
		t.Fatal(err)
	}
	if view.ledger.editor != nil || view.focus != workspaceLedgerPane {
		t.Fatal("cancel left the ledger pane")
	}
	if len(view.ledger.snapshot.Ledger.Tasks) != 3 {
		t.Fatal("canceled draft changed authoritative items")
	}
}

func TestLedgerPlannerMessageIsScopedAndSupportsConversationNewlines(t *testing.T) {
	view, _ := testWorkspace(t, 100, 24)
	openLedgerPane(t, view)
	if err := view.acceptLedger(panelLedgerFixture()); err != nil {
		t.Fatal(err)
	}
	if err := view.key('p', false); err != nil {
		t.Fatal(err)
	}
	for _, value := range []byte("Adjust this item.\nKeep its files.") {
		if err := view.key(value, false); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-view.ledger.requests:
		t.Fatal("newline submitted the planner draft")
	default:
	}
	if err := view.key('\r', false); err != nil {
		t.Fatal(err)
	}
	request := <-view.ledger.requests
	if request.key != 'p' || request.action.TargetID != "panel-first" || request.text != "Adjust this item.\nKeep its files." {
		t.Fatalf("lost selected-item conversation: %+v", request)
	}
	if request.action.Item != nil || strings.Contains(request.text, "panel-second") {
		t.Fatal("planner request included a sibling payload")
	}
}

func TestLedgerRefreshKeepsSaveErrorAndDraftVisible(t *testing.T) {
	view, _ := testWorkspace(t, 100, 24)
	openLedgerPane(t, view)
	snapshot := panelLedgerFixture()
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := view.key('e', false); err != nil {
		t.Fatal(err)
	}
	view.ledger.notice = "Ledger conflict; review your draft before saving again."
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	if view.ledger.editor == nil || view.ledger.feedback != view.ledger.notice {
		t.Fatal("background refresh erased the draft or its save error")
	}
}

func TestLedgerRefreshKeepsManualValidationError(t *testing.T) {
	view, _ := testWorkspace(t, 100, 24)
	openLedgerPane(t, view)
	snapshot := panelLedgerFixture()
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := view.key('e', false); err != nil {
		t.Fatal(err)
	}
	view.ledger.editor.fields[4] = "{"
	if err := view.key(19, false); err != nil {
		t.Fatal(err)
	}
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(view.ledger.feedback, "File responsibilities:") || view.ledger.editor.fields[4] != "{" {
		t.Fatal("refresh erased the manual validation error or draft")
	}
	select {
	case <-view.ledger.requests:
		t.Fatal("invalid manual JSON was submitted")
	default:
	}
}
