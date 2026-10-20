package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ledgerapi"
	"github.com/gryph/omnidex/internal/model"
)

type workspaceLedgerRequest struct {
	key                         byte
	jobID, generation, revision int64
	action                      client.DirectPlanningAction
}

type workspaceLedger struct {
	snapshot        ledgerapi.CodingLedgerRevision
	selected        model.LedgerItemID
	offset          int
	followSelection bool
	opened, loading bool
	feedback        string
	requests        chan workspaceLedgerRequest
}

func (view *chatWorkspace) ledgerRequestLocked(request workspaceLedgerRequest) error {
	select {
	case view.ledger.requests <- request:
		view.ledger.loading = true
		view.ledger.feedback = "Loading ledger…"
	default:
		view.ledger.feedback = "Ledger request in progress"
	}
	return view.drawLocked()
}

func (view *chatWorkspace) ledgerTasksLocked() []model.LedgerTask {
	tasks := append([]model.LedgerTask(nil), view.ledger.snapshot.Ledger.Tasks...)
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].Rank < tasks[j].Rank || tasks[i].Rank == tasks[j].Rank && tasks[i].ID < tasks[j].ID
	})
	return tasks
}

func (view *chatWorkspace) ledgerKeyLocked(key byte) error {
	panel := &view.ledger
	if key == 'r' {
		return view.ledgerRequestLocked(workspaceLedgerRequest{key: 'r'})
	}

	tasks := view.ledgerTasksLocked()
	selected := 0
	for i := range tasks {
		if tasks[i].ID == panel.selected {
			selected = i
		}
	}
	if key == 'j' || key == 'k' {
		if len(tasks) > 0 {
			delta := 1
			if key == 'k' {
				delta = -1
			}
			panel.selected = tasks[max(0, min(len(tasks)-1, selected+delta))].ID
			panel.followSelection = true
		}
		return view.drawLocked()
	}
	if key == 't' {
		return view.ledgerRequestLocked(workspaceLedgerRequest{key: 't'})
	}
	if len(tasks) == 0 || panel.loading {
		return view.drawLocked()
	}
	task := tasks[selected]
	request := workspaceLedgerRequest{key: key, jobID: panel.snapshot.JobID, generation: panel.snapshot.Generation, revision: panel.snapshot.Revision,
		action: client.DirectPlanningAction{TargetID: task.ID, Rationale: "explicit terminal ledger item control"}}
	switch key {
	case 'a':
		request.action.Kind, request.action.ReviewState = model.LedgerMutationReview, model.LedgerReviewAgreed
	case 'd':
		request.action.Kind, request.action.ReviewState = model.LedgerMutationReview, model.LedgerReviewNeedsDiscussion
	case 'x':
		request.action.Kind = model.LedgerMutationRemove
	case 's':
		request.action.Kind = model.LedgerMutationRestore
	case '[', ']':
		neighbor := selected - 1
		if key == ']' {
			neighbor = selected + 1
		}
		if neighbor < 0 || neighbor >= len(tasks) {
			return view.drawLocked()
		}
		rank := tasks[neighbor].Rank
		if key == ']' {
			if rank == int64(^uint64(0)>>1) {
				panel.feedback = "No higher rank available"
				return view.drawLocked()
			}
			rank++
		} else if rank > 0 {
			rank--
		} else if task.ID > tasks[neighbor].ID {
			panel.feedback = "No lower nonnegative rank; move the preceding item down"
			panel.offset = 0
			return view.drawLocked()
		}
		request.action.Kind, request.action.Rank = model.LedgerMutationReorder, &rank
	default:
		return view.drawLocked()
	}
	if panel.snapshot.State != "proposed" {
		panel.feedback = "Ledger is read-only in state " + panel.snapshot.State
		return view.drawLocked()
	}
	if task.Disposition == model.LedgerDispositionTombstoned && key != 's' || task.Disposition != model.LedgerDispositionTombstoned && key == 's' {
		panel.feedback = "Control unavailable for this disposition"
		return view.drawLocked()
	}
	panel.offset = 0
	return view.ledgerRequestLocked(request)
}

func (view *chatWorkspace) acceptLedger(snapshot ledgerapi.CodingLedgerRevision) error {
	view.mu.Lock()
	defer view.mu.Unlock()
	current := view.ledger.snapshot
	same := current.JobID == snapshot.JobID && current.Generation == snapshot.Generation && current.LedgerID == snapshot.LedgerID
	if same && (snapshot.Revision < current.Revision || snapshot.Revision == current.Revision && !reflect.DeepEqual(current.Ledger, snapshot.Ledger)) {
		return fmt.Errorf("ledger response regressed or changed without a revision")
	}

	if !same {
		view.ledger.selected, view.ledger.offset = "", 0
	}
	view.ledger.snapshot = snapshot
	tasks := view.ledgerTasksLocked()
	found := false
	for _, task := range tasks {
		if task.ID == view.ledger.selected {
			found = true
		}
	}
	if !found && len(tasks) > 0 {
		view.ledger.selected = tasks[0].ID
		view.ledger.offset = 0
	}
	view.ledger.loading = false
	view.ledger.feedback = "Server confirmed"
	return view.drawLocked()
}

func (view *chatWorkspace) ledgerLinesLocked(width int) []string {
	panel := &view.ledger
	snapshot := panel.snapshot
	lines := []string{panel.feedback}
	if snapshot.JobID == 0 {
		return append(lines, "No ledger loaded. r refreshes the current job.")
	}
	lines = append(lines, fmt.Sprintf("Job #%d · revision %d · %s", snapshot.JobID, snapshot.Revision, snapshot.State), "", "ORDERED ITEMS")
	if snapshot.State == "frozen" {
		lines = append(lines, "Approved executable revision")
	}
	if len(snapshot.PendingAmendments) > 0 {
		lines = append(lines, "", "PENDING CHANGES · awaiting revision and owner sign-off")
		for _, pending := range snapshot.PendingAmendments {
			lines = append(lines, fmt.Sprintf("From revision %d: %s", pending.Revision, pending.Text))
		}
		label := "REVISION ITEMS"
		if snapshot.State == "frozen" {
			label = "APPROVED ITEMS"
		}
		lines = append(lines, "", label)
	}
	tasks := view.ledgerTasksLocked()
	var selected *model.LedgerTask
	for i, task := range tasks {
		marker := "  "
		if task.ID == panel.selected {
			marker = "> "
			copy := task
			selected = &copy
		}
		label := strings.Join(strings.Fields(task.Goal), " ")
		lines = append(lines, ansi.Truncate(safeConsoleText(fmt.Sprintf("%s%d. %s", marker, i+1, label)), max(1, width), "…"))
	}
	if selected != nil {
		task := *selected
		lines = append(lines, "", "SELECTED ITEM", "Identity: "+string(task.ID), "Goal: "+task.Instructions(), "Review: "+string(task.ReviewState), "Disposition: "+string(task.Disposition), "Rationale: "+task.Rationale,
			fmt.Sprintf("Provenance: %s · %s · revision %d", task.Provenance.Kind, task.Provenance.Reference, task.Provenance.Revision), fmt.Sprintf("Rank: %d", task.Rank))
		for _, group := range []struct {
			label string
			files []string
		}{{"Read", task.ReadFiles}, {"Write", task.WriteFiles}, {"Delete", task.DeleteFiles}} {
			lines = append(lines, group.label+" files:")
			if len(group.files) == 0 {
				lines = append(lines, "  (none)")
			}
			for _, path := range group.files {
				lines = append(lines, "  "+path)
			}
		}
	}
	if len(tasks) == 0 {
		lines = append(lines, "No items")
	}
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, strings.Split(ansi.Hardwrap(safeConsoleText(line), max(1, width), true), "\n")...)
	}
	return wrapped
}

// Navigation reveals the row; manual scrolling remains free to inspect details.
func (view *chatWorkspace) ledgerSelectedRowLocked(width int) int {
	for row, line := range view.ledgerLinesLocked(width) {
		if strings.HasPrefix(line, "> ") {
			return row
		}
	}
	return -1
}

func (view *chatWorkspace) revealLedgerSelectionLocked(width, height int) {
	row := view.ledgerSelectedRowLocked(width)
	if row < 0 || height < 1 {
		return
	}
	if row < view.ledger.offset {
		view.ledger.offset = row
	}
	if row >= view.ledger.offset+height {
		view.ledger.offset = row - height + 1
	}
}
