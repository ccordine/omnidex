package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gryph/omnidex/internal/ledgerapi"
	"github.com/gryph/omnidex/internal/model"
)

func panelLedgerFixture() ledgerapi.CodingLedgerRevision {
	tasks := []model.LedgerTask{}
	for i, id := range []model.LedgerItemID{"panel-first", "panel-second", "panel-removed"} {
		tasks = append(tasks, model.LedgerTask{LedgerItem: model.LedgerItem{ID: id, Rank: int64(i * 10), Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed,
			Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "panel-fixture", Revision: 1}, Rationale: "Keep exact owner scope"}, Goal: "Inspect 界 ledger goal " + string(id), ReadFiles: []string{"cmd/omni/chat_workspace.go"}, WriteFiles: []string{"cmd/omni/chat_workspace_ledger.go"}, DeleteFiles: []string{"old-panel.go"}})
	}
	tasks[2].Disposition, tasks[2].ReviewState = model.LedgerDispositionTombstoned, ""
	return ledgerapi.CodingLedgerRevision{LedgerID: 401, JobID: 801, Generation: 1, Revision: 3, BaseRevision: 2, State: "proposed", Ledger: model.EngineeringLedger{ID: "panel-fixture", Revision: 3, Tasks: tasks}}
}

func openLedgerPane(t *testing.T, view *chatWorkspace) {
	t.Helper()
	for _, key := range []byte{2, '4'} {
		if err := view.key(key, false); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case request := <-view.ledger.requests:
		if request.key != 'r' {
			t.Fatalf("opening requested %q", request.key)
		}
	default:
		t.Fatal("opening did not request authoritative ledger")
	}
}

func TestWorkspaceLedgerDetailsSelectionRefreshAndNarrowResize(t *testing.T) {
	view, _ := testWorkspace(t, 120, 30)
	openLedgerPane(t, view)
	snapshot := panelLedgerFixture()
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	view.mu.Lock()
	text := strings.Join(view.ledgerLinesLocked(110), "\n")
	view.mu.Unlock()
	for _, wanted := range []string{"Job #801", "revision 3", "proposed", "panel-first", "Goal:", "Inspect 界 ledger goal", "Review:", "Disposition:", "Rationale: Keep exact owner scope", "Provenance: owner_turn · panel-fixture · revision 1", "Read files:", "Write files:", "Delete files:", "old-panel.go", "Inspect 界 ledger goal panel-removed"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("missing %q in %s", wanted, text)
		}
	}
	if err := view.key('j', false); err != nil {
		t.Fatal(err)
	}
	if err := view.key('d', true); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{120, 30}, {54, 16}, {24, 8}, {8, 8}, {1, 1}, {80, 24}} {
		if _, _, err := view.Resize(size[0], size[1]); err != nil {
			t.Fatal(err)
		}
		frame, _, _, visible := workspaceFrame(view)
		if visible {
			t.Fatal("ledger stole editor cursor")
		}
		for _, line := range frame {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("overflow at %v: %q", size, line)
			}
		}
	}
	if view.ledger.selected != "panel-second" {
		t.Fatal("resize lost stable selection")
	}
	offset := view.ledger.offset
	snapshot.Revision++
	snapshot.Ledger.Revision++
	snapshot.Ledger.Tasks[1].ReviewState = model.LedgerReviewAgreed
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	if view.ledger.selected != "panel-second" || view.ledger.offset > offset {
		t.Fatal("refresh lost selection/scroll")
	}
	for _, key := range []byte{2, 'z', 2, '?', 27, 2, '3', 2, '4'} {
		if err := view.key(key, false); err != nil {
			t.Fatal(err)
		}
	}
	if view.focus != workspaceLedgerPane || view.ledger.selected != "panel-second" {
		t.Fatal("switch/help lost panel navigation")
	}
}

func TestWorkspaceLedgerDirectControlsCarryExactDisplayedAuthority(t *testing.T) {
	for _, key := range []byte{'a', 'd', 'x', 's', '[', ']'} {
		t.Run(string(key), func(t *testing.T) {
			view, _ := testWorkspace(t, 100, 24)
			openLedgerPane(t, view)
			snapshot := panelLedgerFixture()
			if key == '[' {
				snapshot.Ledger.Tasks[0].Rank = 1
			}
			if err := view.acceptLedger(snapshot); err != nil {
				t.Fatal(err)
			}
			view.ledger.selected = "panel-second"
			if key == 's' {
				view.ledger.selected = "panel-removed"
			}
			if err := view.key(key, false); err != nil {
				t.Fatal(err)
			}
			if view.ledger.loading == false {
				t.Fatal("direct control did not dispatch")
			}
			request := <-view.ledger.requests
			if request.jobID != 801 || request.generation != 1 || request.revision != 3 || request.action.TargetID != view.ledger.selected {
				t.Fatalf("lost exact authority: %+v", request)
			}
			if view.ledger.snapshot.Revision != 3 || view.ledger.snapshot.Ledger.Tasks[1].ReviewState != model.LedgerReviewProposed {
				t.Fatal("optimistic mutation")
			}
			if key == '[' && request.action.Move != -1 {
				t.Fatalf("up rank: %+v", request)
			}
			if key == ']' && request.action.Move != 1 {
				t.Fatalf("down rank: %+v", request)
			}
		})
	}
}

func TestWorkspaceLedgerTerminalReadOnlyStates(t *testing.T) {
	view, _ := testWorkspace(t, 100, 24)
	openLedgerPane(t, view)
	snapshot := panelLedgerFixture()
	snapshot.State = "completed"
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := view.key('x', false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-view.ledger.requests:
		t.Fatal("completed ledger mutation enabled")
	default:
	}
	if !strings.Contains(view.ledger.feedback, "read-only") {
		t.Fatal("missing read-only feedback")
	}
	old := snapshot
	old.Revision--
	if err := view.acceptLedger(old); err == nil {
		t.Fatal("accepted regressed ledger")
	}
}

func TestWorkspaceLedgerOrderUsesServerRankAndIdentity(t *testing.T) {
	view, _ := testWorkspace(t, 100, 24)
	snapshot := panelLedgerFixture()
	snapshot.Ledger.Tasks[0].Rank = 10
	snapshot.Ledger.Tasks[1].Rank = 0
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	view.mu.Lock()
	tasks := view.ledgerTasksLocked()
	view.mu.Unlock()
	if fmt.Sprint(tasks[0].ID) != "panel-second" {
		t.Fatal("ignored server rank")
	}
}

func TestWorkspaceLedgerKeepsActualEditorDraftCursorStreamAndNavigation(t *testing.T) {
	console, input, reads, done := workspaceEditor(t)
	writeComposerKeys(t, input, reads, "ab\x1b[DX")
	view := console.workspace
	openLedgerPane(t, view)
	if err := view.acceptLedger(panelLedgerFixture()); err != nil {
		t.Fatal(err)
	}
	if err := view.key('j', false); err != nil {
		t.Fatal(err)
	}
	if err := view.key('d', true); err != nil {
		t.Fatal(err)
	}
	if err := console.WriteOutput("omni> streamed during ledger inspection\n"); err != nil {
		t.Fatal(err)
	}
	if err := view.Observe("Review", "fixture-model", "kept activity"); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{38, 12}, {120, 28}} {
		columns, rows, err := view.Resize(size[0], size[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := console.terminal.SetSize(columns, rows); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := panelLedgerFixture()
	snapshot.Revision++
	snapshot.Ledger.Revision++
	if err := view.acceptLedger(snapshot); err != nil {
		t.Fatal(err)
	}
	for _, key := range []byte{2, '3', 2, 'z', 2, '4', 2, '?', 27, 2, '1'} {
		if err := view.key(key, false); err != nil {
			t.Fatal(err)
		}
	}
	if view.ledger.selected != "panel-second" || len(view.events) != 1 {
		t.Fatal("lost selection or stream")
	}
	// Insert at the retained middle cursor. Returning the exact editor line is
	// stronger evidence than just finding a draft string in a rendered frame.
	writeComposerKeys(t, input, reads, "Y")
	finishScreenComposer(t, input, done, "aXYb")
}

func TestWorkspaceLedgerReorderBoundaries(t *testing.T) {
	for _, test := range []struct {
		name                  string
		selected              model.LedgerItemID
		key                   byte
		firstRank, secondRank int64
		move                  int
	}{
		{name: "first cannot move up", selected: "panel-first", key: '[', firstRank: 0, secondRank: 10},
		{name: "zero tie can move up", selected: "panel-second", key: '[', firstRank: 0, secondRank: 0, move: -1},
		{name: "last cannot move down", selected: "panel-removed", key: ']', firstRank: 0, secondRank: 10},
		{name: "maximum rank can move down", selected: "panel-first", key: ']', firstRank: 0, secondRank: int64(^uint64(0) >> 1), move: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			view, _ := testWorkspace(t, 100, 24)
			openLedgerPane(t, view)
			snapshot := panelLedgerFixture()
			snapshot.Ledger.Tasks = snapshot.Ledger.Tasks[:2]
			snapshot.Ledger.Tasks[0].Rank = test.firstRank
			snapshot.Ledger.Tasks[1].Rank = test.secondRank
			if test.selected == "panel-removed" {
				test.selected = "panel-second"
			}
			if err := view.acceptLedger(snapshot); err != nil {
				t.Fatal(err)
			}
			view.ledger.selected = test.selected
			if err := view.key(test.key, false); err != nil {
				t.Fatal(err)
			}
			select {
			case request := <-view.ledger.requests:
				if test.move == 0 || request.action.Move != test.move {
					t.Fatalf("unexpected move %+v", request)
				}
			default:
				if test.move != 0 {
					t.Fatal("valid move was blocked by a rank boundary")
				}
			}
		})
	}
}

func TestWorkspaceLedgerLongGoalsRenderedNavigation(t *testing.T) {
	for _, size := range [][2]int{{32, 12}, {120, 24}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			view, _ := testWorkspace(t, size[0], size[1])
			openLedgerPane(t, view)
			snapshot := panelLedgerFixture()
			snapshot.Ledger.Tasks = nil
			for i := 0; i < 45; i++ {
				task := panelLedgerFixture().Ledger.Tasks[0]
				task.ID = model.LedgerItemID(fmt.Sprintf("identity-%02d", i))
				task.Rank = int64(i * 10)
				task.Goal = fmt.Sprintf("Recognizable %02d goal\n\n  second paragraph with exact whitespace\nthird paragraph %s", i, strings.Repeat("long detail ", 12))
				snapshot.Ledger.Tasks = append(snapshot.Ledger.Tasks, task)
			}
			if err := view.acceptLedger(snapshot); err != nil {
				t.Fatal(err)
			}
			assertSelected := func(i int) {
				t.Helper()
				frame, _, _, _ := workspaceFrame(view)
				body := strings.Join(frame[2:len(frame)-2], "\n")
				if !strings.Contains(body, fmt.Sprintf("> %d. Recognizable %02d", i+1, i)) {
					t.Fatalf("selected row %d hidden:\n%s", i, body)
				}
				for _, line := range frame {
					if ansi.StringWidth(line) > view.width {
						t.Fatalf("overflow: %q", line)
					}
				}
			}
			assertSelected(0)
			for _, key := range []byte{'j', 'k'} {
				for step := 1; step < 45; step++ {
					i := step
					if key == 'k' {
						i = 44 - step
					}
					if err := view.key(key, false); err != nil {
						t.Fatal(err)
					}
					assertSelected(i)
					if i == 22 || i == 44 || i == 0 {
						for _, resized := range [][2]int{{40, 10}, {110, 30}, size} {
							if _, _, err := view.Resize(resized[0], resized[1]); err != nil {
								t.Fatal(err)
							}
							assertSelected(i)
						}
					}
				}
			}
			view.mu.Lock()
			lines := view.ledgerLinesLocked(28)
			view.mu.Unlock()
			firstRow := 0
			for index, line := range lines {
				if line == "ORDERED ITEMS" {
					firstRow = index + 1
					break
				}
			}
			for i := 0; i < 45; i++ {
				if !strings.Contains(lines[firstRow+i], fmt.Sprintf("Recognizable %02d", i)) || ansi.StringWidth(lines[firstRow+i]) > 28 {
					t.Fatalf("unrecognizable or unbounded row: %q", lines[firstRow+i])
				}
			}
			expected := strings.Split(ansi.Hardwrap(safeConsoleText("Goal: "+snapshot.Ledger.Tasks[0].Goal), 28, true), "\n")
			if !strings.Contains(strings.Join(lines, "\n"), strings.Join(expected, "\n")) {
				t.Fatal("selected goal changed")
			}
			// Inspect every rendered detail row using manual one-line scrolling.
			seen := ""
			for i := 0; i < len(lines); i++ {
				frame, _, _, _ := workspaceFrame(view)
				seen += strings.Join(frame[2:len(frame)-2], "\n") + "\n"
				view.mu.Lock()
				view.scrollLocked(-1)
				view.mu.Unlock()
			}
			for _, detail := range []string{"Identity: identity-00", "Goal: Recognizable 00", "second paragraph", "third paragraph", "Rationale:", "Provenance:", "Read files:", "Write files:", "Delete files:"} {
				if !strings.Contains(seen, detail) {
					t.Fatalf("detail inaccessible in viewport: %s", detail)
				}
			}
			view.mu.Lock()
			detailLines := view.ledgerLinesLocked(max(1, view.width-4))
			view.mu.Unlock()
			inDetails := false
			for _, line := range detailLines {
				if line == "SELECTED ITEM" {
					inDetails = true
				}
				if inDetails && strings.TrimSpace(line) != "" && !strings.Contains(seen, line) {
					t.Fatalf("exact detail row inaccessible in viewport: %q", line)
				}
			}
			offset := view.ledger.offset
			if _, _, err := view.Resize(80, 14); err != nil {
				t.Fatal(err)
			}
			if view.ledger.offset == 0 || offset == 0 {
				t.Fatal("manual detail scroll reset on resize")
			}
			if err := view.key('j', false); err != nil {
				t.Fatal(err)
			}
			assertSelected(1)
		})
	}
}
