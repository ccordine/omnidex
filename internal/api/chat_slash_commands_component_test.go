package api

import (
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"golang.org/x/net/html"
)

func TestSlashCommandDOMIdentitiesBelongToTheRenderer(t *testing.T) {
	channel := model.Channel{
		ID: "slash-options", Scope: model.ChannelScopeUser, Mode: model.ChannelModeRoleplay,
		Name: "Commands", WorkspaceRoot: t.TempDir(),
		RoleplayViewpointCharacterID: "rpc_11111111111111111111111111111111",
	}
	const itemAction = `/give "Instrument 🔭 & lens"`
	projection := roleplay.SimulationSlashCommandProjection{
		Schema:  roleplay.SimulationSlashCommandProjectionSchemaV1,
		WorldID: "rpw_11111111111111111111111111111111", SceneID: "rps_11111111111111111111111111111111",
		SceneRevision: 1, ActiveCharacterID: string(channel.RoleplayViewpointCharacterID), ActiveCharacterName: "Observer",
		Commands: []roleplay.SimulationSlashCommand{
			{Kind: roleplay.SimulationSlashCommandInteraction, Key: "wait", Insertion: "/wait", Display: "/wait",
				Label: "Wait", Description: "Wait for a moment.", CursorUTF16: 5, DisplayOrder: 0},
			{Kind: roleplay.SimulationSlashCommandGive, Key: "give", Insertion: itemAction, Display: itemAction,
				Label: "Give instrument", Description: "Give the instrument to the observer.",
				CursorUTF16: len(utf16.Encode([]rune(itemAction))), DisplayOrder: 1},
		},
	}
	component, err := renderChatSlashCommandsComponent(channel, &projection)
	if err != nil {
		t.Fatal(err)
	}
	document, err := html.Parse(strings.NewReader(component.HTML.Bundle))
	if err != nil {
		t.Fatal(err)
	}
	var options []map[string]string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "button" {
			attributes := make(map[string]string)
			for _, attribute := range node.Attr {
				attributes[attribute.Key] = attribute.Val
			}
			options = append(options, attributes)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	if component.CommandCount != 2 || len(options) != 2 {
		t.Fatalf("rendered options = %#v", options)
	}
	for index, expectedID := range []string{"slash-command-option-0", "slash-command-option-1"} {
		if options[index]["id"] != expectedID || options[index]["role"] != "option" ||
			options[index]["data-slash-command"] != projection.Commands[index].Insertion {
			t.Fatalf("option %d lost its DOM identity or exact insertion: %#v", index, options[index])
		}
	}
	duplicate := projection.Commands[0]
	duplicate.DisplayOrder = 1
	projection.Commands[1] = duplicate
	if _, err := renderChatSlashCommandsComponent(channel, &projection); err == nil || !strings.Contains(err.Error(), "repeats exact syntax") {
		t.Fatalf("duplicate syntax was accepted: %v", err)
	}
}
