package api

import (
	"os"
	"strings"
	"testing"
)

func TestSlashPaletteBrowserCoordinatesOnlyServerRenderedCommands(t *testing.T) {
	t.Parallel()
	coordinator := readSlashPaletteBrowserSource(t, "web/src/lib/chat_slash_palette_coordinator.ts")
	apiSource := readSlashPaletteBrowserSource(t, "web/src/lib/chat_slash_palette_api.ts")
	panel := readSlashPaletteBrowserSource(t, "web/panels/chat.html")

	for _, required := range []string{
		"fetchSlashCommandComponent(", "renderComponentBundle(component.html.bundle)",
		"slashCommandPrefix", "slashCommandCursor", "generation",
	} {
		if !strings.Contains(coordinator, required) {
			t.Errorf("slash-command coordinator lacks %q", required)
		}
	}
	for _, required := range []string{
		`/v1/ui/chat/slash-commands?`, `new URLSearchParams({ channel_id: channelID })`,
	} {
		if !strings.Contains(apiSource, required) {
			t.Errorf("slash-command component API lacks %q", required)
		}
	}
	for _, required := range []string{
		`data-recyclr-sink="slash-command-options"`, `input->chat#composerInput`,
		`keydown->chat#slashCommandKeydown`, `aria-autocomplete="list"`,
	} {
		if !strings.Contains(panel, required) {
			t.Errorf("chat composer lacks slash-command affordance %q", required)
		}
	}

	for _, forbidden := range []string{
		".innerHTML", "insertAdjacentHTML(", "document.createElement(", "<template", "replaceChildren(",
		`'/give`, `"/give`, `'/take`, `"/take`, `'/research`, `"/research`,
		"sendChannelMessage(", "jsonRequest(", "method: \"POST\"",
	} {
		if strings.Contains(coordinator, forbidden) || strings.Contains(apiSource, forbidden) {
			t.Errorf("slash-command browser owns forbidden authority %q", forbidden)
		}
	}
}

func readSlashPaletteBrowserSource(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
