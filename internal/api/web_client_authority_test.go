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

func TestScrumControllerUsesServerCardsByColumn(t *testing.T) {
	source := readFrontendSource(t, "web/src/controllers/scrum_controller.ts")
	required := []string{
		"private cardsByCol: Record<string, ScrumCard[]> = {};",
		"const previousCardsByCol = previousBoard ? this.cardsByCol : null;",
		"this.cardsByCol = payload.cards_by_col ?? {};",
		"const cardsByCol = this.cardsByCol;",
	}
	for _, snippet := range required {
		if !strings.Contains(source, snippet) {
			t.Fatalf("scrum controller must render from server cards_by_col; missing %q", snippet)
		}
	}
	if strings.Contains(source, "groupCardsByColumn(") {
		t.Fatalf("scrum controller must not regroup board cards client-side")
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
