package api

import (
	"os"
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
		"card.column = result.column",
		"const card = await moveScrumCard(result.cardID",
	}
	for _, snippet := range forbidden {
		if strings.Contains(source, snippet) {
			t.Fatalf("scrum controller must wait for server board refresh after drag move; found %q", snippet)
		}
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
