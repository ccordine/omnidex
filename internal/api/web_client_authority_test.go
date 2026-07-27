package api

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestScrumDragDoesNotMoveCardDOMBeforeServerRefresh(t *testing.T) {
	source := readFrontendSource(t, "web/src/lib/scrum_drag.ts")

	forbidden := []string{
		"commitDrop(",
		"replaceWith(session.cardEl)",
		"session.cardEl.replaceWith",
		"session.cardEl.dataset.scrumColumn =",
		"insertBefore(session.placeholder",
		"appendChild(session.placeholder",
		"insertBefore(session.cardEl",
		"appendChild(session.cardEl",
	}
	for _, snippet := range forbidden {
		if strings.Contains(source, snippet) {
			t.Fatalf("scrum drag must not move card DOM before server refresh; found %q", snippet)
		}
	}
}

func TestScrumControllerDoesNotMutateDraggedCardColumnBeforeServerRefresh(t *testing.T) {
	source := readFrontendSource(t, "web/src/controllers/scrum_controller.ts")

	forbidden := []string{
		"applyDragResult(",
		"upsertCard(",
		"card.column = result.column",
		"this.board.cards[",
		"const card = await moveScrumCard(result.cardID",
		"this.board.cards.push(",
		"this.setActiveColumn(destinationColumn)",
		"this.activeCardTab = \"channel\"",
		"this.persistCardTab(\"channel\")",
	}
	for _, snippet := range forbidden {
		if strings.Contains(source, snippet) {
			t.Fatalf("scrum controller must wait for server board refresh after drag move; found %q", snippet)
		}
	}
}

func TestScrumControllerDoesNotSwitchColumnAfterServerMove(t *testing.T) {
	source := readFrontendSource(t, "web/src/controllers/scrum_controller.ts")
	pattern := regexp.MustCompile(`(?s)private async requestServerCardMove\(.*?this\.setActiveColumn\(`)
	if pattern.MatchString(source) {
		t.Fatalf("scrum controller must refresh the current server viewport after a move, not switch columns")
	}
}

func TestScrumControllerRequiresServerRenderedBoardBundle(t *testing.T) {
	source := readFrontendSource(t, "web/src/controllers/scrum_controller.ts")
	required := []string{
		"await this.applyServerBundle(payload.html?.bundle);",
		"Scrum board response did not include its required server-rendered Recyclr bundle.",
	}
	for _, snippet := range required {
		if !strings.Contains(source, snippet) {
			t.Fatalf("scrum controller must require the server-rendered board bundle; missing %q", snippet)
		}
	}
	for _, forbidden := range []string{
		"private cardsByCol:",
		"groupCardsByColumn(",
		"renderScrumBoard(",
		"renderScrumColumn(",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("client-rendered board fallback remains: %q", forbidden)
		}
	}
}

func TestAdminDataSourcesUseFocusedController(t *testing.T) {
	admin := readFrontendSource(t, "web/src/controllers/admin_controller.ts")
	dataSources := readFrontendSource(t, "web/src/lib/data_sources_render.ts")
	if strings.Contains(admin, "async loadDataSources(") || strings.Contains(admin, "pollDataSourceJob") {
		t.Fatal("admin controller still owns the data-source runtime")
	}
	if !strings.Contains(dataSources, "admin-data-sources#askDataSource") {
		t.Fatal("data-source interactions are not wired to the focused controller")
	}
	if strings.Contains(dataSources, "admin#askDataSource") {
		t.Fatal("legacy admin data-source action remains")
	}
}

func TestProjectsControllerDelegatesFocusedModalWorkflows(t *testing.T) {
	source := readFrontendSource(t, "web/src/controllers/projects_controller.ts")
	if lines := strings.Count(source, "\n") + 1; lines >= 800 {
		t.Fatalf("projects controller has %d lines; focused controllers must stay below 800", lines)
	}
	for _, required := range []string{"ProjectBrowserCoordinator", "ProjectDebuggerCoordinator"} {
		if !strings.Contains(source, required) {
			t.Errorf("projects controller does not delegate to %s", required)
		}
	}
	for _, forbidden := range []string{"observeRealtimeJob({", "browseDirectory(path)", "renderProjectDebuggerModal({"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("projects controller still owns focused modal workflow %q", forbidden)
		}
	}
}

func TestChatControllerDelegatesOperationalPanels(t *testing.T) {
	source := readFrontendSource(t, "web/src/controllers/chat_controller.ts")
	if lines := strings.Count(source, "\n") + 1; lines >= 600 {
		t.Fatalf("chat controller has %d lines; focused controllers must stay below 600", lines)
	}
	for _, required := range []string{
		"ChatTargetsController",
		"ChatPanelCoordinator",
		"ChatExecutionCoordinator",
		"ChatJobsCoordinator",
		"ChatMemoryCoordinator",
		"ChatSystemCoordinator",
		"recordChatJobProgress",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("chat controller does not delegate to %s", required)
		}
	}
	for _, forbidden := range []string{
		`fetch("/v1/ui/session"`,
		"fetch(`/v1/ui/panel?",
		"pendingJobCompletion",
		"jobRefreshInFlight",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("chat controller still owns delegated orchestration %q", forbidden)
		}
	}
}

func TestChatCollaboratorsStayFocusedAndHaveNoDuplicateAuthority(t *testing.T) {
	for _, path := range []string{
		"web/src/controllers/chat_targets_controller.ts",
		"web/src/lib/chat_panel_coordinator.ts",
		"web/src/lib/chat_execution_coordinator.ts",
	} {
		source := readFrontendSource(t, path)
		if lines := strings.Count(source, "\n") + 1; lines > 300 {
			t.Errorf("%s has %d lines; chat collaborators must stay at or below 300", path, lines)
		}
	}

	panel := readFrontendSource(t, "web/src/lib/chat_panel_coordinator.ts")
	if strings.Contains(panel, "/v1/ui/session") {
		t.Fatal("panel coordinator duplicates the server-authoritative panel session write")
	}
	execution := readFrontendSource(t, "web/src/lib/chat_execution_coordinator.ts")
	for _, forbidden := range []string{"setInterval(", `|| "Completed."`, "jobRefreshInFlight"} {
		if strings.Contains(execution, forbidden) {
			t.Errorf("chat execution coordinator contains polling, fallback, or superseded state %q", forbidden)
		}
	}
	transcript := readFrontendSource(t, "web/src/lib/transcript_store.ts")
	if strings.Contains(transcript, "catch {\n      return []") {
		t.Fatal("transcript storage still silently replaces corruption with an empty transcript")
	}
}

func TestProjectPlannerUsesServerConfirmedRealtimeState(t *testing.T) {
	controller := readFrontendSource(t, "web/src/controllers/project_chat_controller.ts")
	recyclr := readFrontendSource(t, "web/src/controllers/recyclr_controller.ts")
	for _, required := range []string{
		`document.addEventListener("omni:project-planning-updated"`,
		"this.reloadPending = true",
		`this.setStatus("Saved · live sync degraded", "error")`,
	} {
		if !strings.Contains(controller, required) {
			t.Errorf("project planner realtime path missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`created_at: new Date().toISOString()`,
		`new CustomEvent("omni:scrum-refresh"`,
		"catch {\n      this.modelOptions = []",
	} {
		if strings.Contains(controller, forbidden) {
			t.Errorf("project planner contains client-authoritative or silent path %q", forbidden)
		}
	}
	if !strings.Contains(recyclr, `"project-planning-updated": "omni:project-planning-updated"`) {
		t.Fatal("page-scoped Recyclr does not bridge project planning updates")
	}
}

func TestAIControlUsesTypedRealtimeEventsWithoutPolling(t *testing.T) {
	shell := readFrontendSource(t, "web/src/controllers/shell_controller.ts")
	recyclr := readFrontendSource(t, "web/src/controllers/recyclr_controller.ts")
	for _, required := range []string{
		`document.addEventListener("omni:ai-control-updated"`,
		`document.addEventListener("omni:job-progress"`,
		"this.applyAIControlPayload(state as AIControlPayload)",
	} {
		if !strings.Contains(shell, required) {
			t.Errorf("AI control realtime path missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"aiControlTimer",
		"setInterval(() => void this.loadAIControl()",
		`new CustomEvent("omni:scrum-refresh"`,
	} {
		if strings.Contains(shell, forbidden) {
			t.Errorf("AI control contains polling or duplicate refresh path %q", forbidden)
		}
	}
	if !strings.Contains(recyclr, `"ai-control-updated": "omni:ai-control-updated"`) {
		t.Fatal("page-scoped Recyclr does not bridge AI control updates")
	}
}

func readFrontendSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
