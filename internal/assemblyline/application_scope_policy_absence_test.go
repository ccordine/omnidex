package assemblyline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequirementInventoryHasNoOptionalScopePolicy(t *testing.T) {
	for _, request := range []string{
		"The software lets a reader bookmark a passage.",
		"The software lets an operator silence an alarm.",
	} {
		t.Run(request, func(t *testing.T) {
			context, err := BootstrapApplicationContext(request)
			if err != nil {
				t.Fatal(err)
			}
			input := ApplicationRequirementInventoryInput{UserRequest: request, Context: context}
			job, err := NewApplicationRequirementInventoryJob(input)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(prompt, request) || !strings.Contains(prompt, "Do not add optional product scope.") {
				t.Fatalf("inventory lacks its exact request-only boundary: %q", prompt)
			}
			for _, forbidden := range []string{"usefully expand the objective", "useful consequences", "scope_mode"} {
				if strings.Contains(prompt, forbidden) {
					t.Fatalf("inventory still invites scope expansion through %q", forbidden)
				}
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(job.Payload, &fields); err != nil {
				t.Fatal(err)
			}
			fields["scope_mode"] = json.RawMessage(`"expansive"`)
			job.Payload, err = json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := RenderPortableJob(job); err == nil {
				t.Fatal("inventory accepted the removed scope policy")
			}
		})
	}
}

func TestCodingScopeControlsAndAnnotationsAreAbsent(t *testing.T) {
	for _, directory := range []string{".", "../worker", "../queue", "../model", "../config", "../runtime", "../scrum", "../../cmd/omni"} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"CodingScopeMode", "codingScopeMode", "CodingPlanAnnotation", "applicationRequirementInventoryScopeGuidance"} {
				if strings.Contains(string(source), forbidden) {
					t.Errorf("%s retains %s", path, forbidden)
				}
			}
		}
	}
}
