package api

import (
	"os"
	"strings"
	"testing"
)

func TestBrowseProductionDoesNotExpandProjectInventory(t *testing.T) {
	for _, path := range []string{"browse_handlers.go", "ui_project_modal_component.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "ListProjects(") {
			t.Fatalf("%s must authorize a browse target without loading the project inventory", path)
		}
	}
}
