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
	controller := readFrontendSource(t, "web/src/controllers/admin_data_sources_controller.ts")
	component, err := os.ReadFile("ui_admin_data_sources.go")
	if err != nil {
		t.Fatalf("read server data-source component: %v", err)
	}
	if strings.Contains(admin, "async loadDataSources(") || strings.Contains(admin, "pollDataSourceJob") {
		t.Fatal("admin controller still owns the data-source runtime")
	}
	for _, required := range []string{"admin-data-sources#loadDataSourceSchema", "admin-data-sources#runDataSourceQuery"} {
		if !strings.Contains(string(component), required) {
			t.Errorf("server data-source component is missing focused action %q", required)
		}
	}
	for _, forbidden := range []string{"askDataSource", "exploreDataSource", "updateDataSourceChart"} {
		if strings.Contains(controller, forbidden) {
			t.Errorf("retired data-source inference/chart client capability remains: %q", forbidden)
		}
	}
}

func TestProjectsControllerDelegatesFocusedModalWorkflows(t *testing.T) {
	source := readFrontendSource(t, "web/src/controllers/projects_controller.ts")
	if lines := strings.Count(source, "\n") + 1; lines >= 800 {
		t.Fatalf("projects controller has %d lines; focused controllers must stay below 800", lines)
	}
	for _, required := range []string{"ProjectBrowserCoordinator"} {
		if !strings.Contains(source, required) {
			t.Errorf("projects controller does not delegate to %s", required)
		}
	}
	for _, forbidden := range []string{"ProjectDebuggerCoordinator", "observeRealtimeJob({", "browseDirectory(path)", "renderProjectDebuggerModal({"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("projects controller still owns focused modal workflow %q", forbidden)
		}
	}
}

func TestChatControllerDelegatesOperationalPanels(t *testing.T) {
	paths := []string{
		"web/src/controllers/chat_controller.ts",
		"web/src/controllers/chat_runtime_controller.ts",
		"web/src/controllers/chat_view_controller.ts",
	}
	var sources []string
	for _, path := range paths {
		source := readFrontendSource(t, path)
		if lines := strings.Count(source, "\n") + 1; lines > 300 {
			t.Errorf("%s has %d lines; focused chat controllers must stay at or below 300", path, lines)
		}
		sources = append(sources, source)
	}
	source := strings.Join(sources, "\n")
	if !strings.Contains(sources[0], "ChatRuntimeController") {
		t.Error("chat controller does not delegate lifecycle authority to ChatRuntimeController")
	}
	for _, required := range []string{
		"ChatTargetsController",
		"ChatPanelCoordinator",
		"ChatExecutionCoordinator",
		"ChatJobsCoordinator",
		"ChatMemoryCoordinator",
		"ChatSystemCoordinator",
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
}

func TestOrdinaryWebChatUsesOnlyServerAuthoritativeChannels(t *testing.T) {
	for _, path := range []string{
		"web/src/controllers/chat_controller.ts",
		"web/src/controllers/chat_runtime_controller.ts",
		"web/src/controllers/chat_view_controller.ts",
		"web/src/lib/chat_channel_coordinator.ts",
		"web/src/lib/chat_execution_coordinator.ts",
	} {
		source := readFrontendSource(t, path)
		for _, forbidden := range []string{
			`fetch("/v1/jobs"`, `pipeline: "chat"`, "local-transcript",
			"restoreObjectiveTranscript", "restoreLocalTranscript", "TranscriptStore",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("ordinary chat fallback %q remains in %s", forbidden, path)
			}
		}
	}
	for _, path := range []string{
		"web/src/lib/transcript_store.ts",
		"web/src/lib/transcript_store.test.ts",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("browser-authoritative transcript surface remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", path, err)
		}
	}
	channel := readFrontendSource(t, "web/src/lib/chat_channel_coordinator.ts")
	for _, required := range []string{"sendChannelMessage(", "workspaceRoot()"} {
		if !strings.Contains(channel, required) {
			t.Errorf("channel-only chat path is missing %q", required)
		}
	}
}

func TestRemovedProjectPlannerHasNoClientCapabilitySurface(t *testing.T) {
	for _, path := range []string{
		"web/src/controllers/project_chat_controller.ts",
		"web/src/lib/project_chat_api.ts",
		"web/src/lib/project_chat_render.ts",
		"web/src/lib/project_debugger_coordinator.ts",
		"web/src/lib/project_debugger_render.ts",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("removed direct-inference client surface remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", path, err)
		}
	}
	recyclr := readFrontendSource(t, "web/src/controllers/recyclr_controller.ts")
	if strings.Contains(recyclr, "project-planning-updated") {
		t.Fatal("frontend still subscribes to removed project-planning generation state")
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
