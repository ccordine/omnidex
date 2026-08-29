package api

import (
	"os"
	"strings"
	"testing"
)

func TestRoleplayBrowserCoordinatesOnlyServerComponents(t *testing.T) {
	t.Parallel()
	coordinator := readRoleplayAuthoritySource(t, "web/src/lib/chat_roleplay_coordinator.ts") +
		readRoleplayAuthoritySource(t, "web/src/lib/chat_roleplay_support.ts")
	for _, required := range []string{
		"fetchRoleplayComponent(", "renderComponentBundle(component.html.bundle)",
		"setComposerAvailable(component.configured)", "form.setAttribute(\"aria-busy\", \"true\")",
	} {
		if !strings.Contains(coordinator, required) {
			t.Errorf("roleplay coordinator lacks server-authority token %q", required)
		}
	}
	for _, forbidden := range []string{
		".innerHTML", "insertAdjacentHTML(", "document.createElement(", "<template", "ApplySimulationTransition",
	} {
		if strings.Contains(coordinator, forbidden) {
			t.Errorf("roleplay coordinator owns component or transition authority %q", forbidden)
		}
	}
}

func TestRoleplayHTTPHasNoSecondActionTransport(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"roleplay_simulation_store.go",
		"roleplay_simulation_mutation_http.go",
		"roleplay_simulation_definition_http.go",
		"roleplay_simulation_capability_http.go",
		"web/src/lib/roleplay_api.ts",
	} {
		source := readRoleplayAuthoritySource(t, path)
		for _, forbidden := range []string{
			"ApplySimulationTransition", "SimulationTransitionRequest", "/roleplay/actions", "/roleplay/commands",
			"/roleplay/research", "LoadRoleplayResearchTurn", "ExecuteResearch", "RunResearch",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s exposes second roleplay action transport %q", path, forbidden)
			}
		}
	}
}

func TestRoleplayUIProjectionUsesOneRepeatableReadDatabaseSnapshot(t *testing.T) {
	t.Parallel()
	httpSource := readRoleplayAuthoritySource(t, "roleplay_simulation_http.go")
	for _, required := range []string{
		"ProjectSimulationUI(", "CharactersOffset: page.Characters", "PersonasOffset: page.Personas",
		"ItemTemplatesOffset: page.ItemTemplates",
	} {
		if !strings.Contains(httpSource, required) {
			t.Errorf("roleplay UI projection lacks one snapshot boundary %q", required)
		}
	}
	for _, forbidden := range []string{
		"ListSimulationCharactersPage(", "ListPersonaPage(", "ListSceneParticipantsPage(",
		"ListViewpointMetersPage(", "ListInventoryPage(", "ListInteractionCommandsPage(",
	} {
		if strings.Contains(httpSource, forbidden) {
			t.Errorf("roleplay HTTP projection assembles independent database moments with %q", forbidden)
		}
	}
	projection := readRoleplayAuthoritySource(t, "../roleplay/simulation_ui_projection.go") +
		readRoleplayAuthoritySource(t, "../roleplay/simulation_ui_projection_configured.go")
	for _, required := range []string{
		"pgx.RepeatableRead", "pgx.ReadOnly", "loadSimulationCharactersPage(",
		"loadSceneParticipantsPage(", "loadItemTemplatesPage(", "tx.Commit(ctx)",
	} {
		if !strings.Contains(projection, required) {
			t.Errorf("roleplay snapshot projection lacks %q", required)
		}
	}
}

func readRoleplayAuthoritySource(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
