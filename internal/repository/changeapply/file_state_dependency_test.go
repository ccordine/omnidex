package changeapply_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestDeletionRejectsRemainingTypedReferences(t *testing.T) {
	tests := []struct {
		name     string
		obsolete string
		consumer string
		relation string
	}{
		{
			name:     "type use",
			obsolete: "package references\n\ntype Legacy struct { Value int }\n",
			consumer: "package references\n\nfunc Consume(value Legacy) int { return value.Value }\n",
			relation: "uses_type",
		},
		{
			name:     "value use",
			obsolete: "package references\n\nconst LegacyDefault = 7\n",
			consumer: "package references\n\nfunc Consume() int { return LegacyDefault }\n",
			relation: "uses_value",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixture(t, map[string]fixtureEntry{
				"go.mod":      {content: "module example.com/references\n\ngo 1.24\n", mode: 0o644},
				"obsolete.go": {content: test.obsolete, mode: 0o644},
				"consumer.go": {content: test.consumer, mode: 0o644},
			})
			if !analysisHasEdgeKind(fixture, test.relation) {
				t.Fatalf("fixture has no %s authority", test.relation)
			}
			_, err := planFixtureDeletion(t, fixture, "obsolete.go")
			if err == nil || !strings.Contains(err.Error(), "remains referenced") {
				t.Fatalf("typed reference deletion error=%v", err)
			}
		})
	}
}

func TestDeletionRejectsBlankImportRegistrationDependency(t *testing.T) {
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod": {content: "module example.com/registration\n\ngo 1.24\n", mode: 0o644},
		"registry.go": {
			content: "package registration\n\nimport _ \"example.com/registration/plugin\"\n",
			mode:    0o644,
		},
		"plugin/retained.go":     {content: "package plugin\n\nfunc Retained() {}\n", mode: 0o644},
		"plugin/registration.go": {content: "package plugin\n\nfunc init() {}\n", mode: 0o644},
	})
	if !analysisHasEdgeKind(fixture, "registers") {
		t.Fatal("fixture has no exact registration authority")
	}
	_, err := planFixtureDeletion(t, fixture, "plugin/registration.go")
	if err == nil || !strings.Contains(err.Error(), "remains referenced") {
		t.Fatalf("registration dependency deletion error=%v", err)
	}
}

func TestDeletionRejectsExactEmbedBuildInputDependency(t *testing.T) {
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod": {content: "module example.com/buildinput\n\ngo 1.24\n", mode: 0o644},
		"source.go": {
			content: "package buildinput\n\nimport _ \"embed\"\n\n//go:embed assets/obsolete.go\nvar embedded string\n",
			mode:    0o644,
		},
		"assets/retained.go": {content: "package assets\n\nfunc Retained() {}\n", mode: 0o644},
		"assets/obsolete.go": {content: "package assets\n\nfunc Obsolete() {}\n", mode: 0o644},
	})
	if !analysisHasEdgeKind(fixture, "embeds") {
		t.Fatal("fixture has no exact embedded build-input authority")
	}
	_, err := planFixtureDeletion(t, fixture, "assets/obsolete.go")
	if err == nil || !strings.Contains(err.Error(), "remains referenced") {
		t.Fatalf("embedded build-input deletion error=%v", err)
	}
}

func TestDeletionRejectsLastIndexedGoBuildMember(t *testing.T) {
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":           {content: "module example.com/buildmember\n\ngo 1.24\n", mode: 0o644},
		"retained.go":      {content: "package buildmember\n\nfunc Retained() {}\n", mode: 0o644},
		"orphan/orphan.go": {content: "package orphan\n\nfunc Obsolete() {}\n", mode: 0o644},
	})
	_, err := planFixtureDeletion(t, fixture, "orphan/orphan.go")
	if err == nil || !strings.Contains(err.Error(), "last indexed Go build member") {
		t.Fatalf("last build-member deletion error=%v", err)
	}
}

func TestDeletionDoesNotTreatTestSourceAsProductionBuildMembership(t *testing.T) {
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":         {content: "module example.com/buildclass\n\ngo 1.24\n", mode: 0o644},
		"source.go":      {content: "package buildclass\n\nfunc Obsolete() {}\n", mode: 0o644},
		"source_test.go": {content: "package buildclass\n\nimport \"testing\"\n\nfunc TestPlaceholder(t *testing.T) {}\n", mode: 0o644},
	})
	_, err := planFixtureDeletion(t, fixture, "source.go")
	if err == nil || !strings.Contains(err.Error(), "last indexed Go build member") {
		t.Fatalf("test-only build membership deletion error=%v", err)
	}
}

func TestDeletionRequiresCompleteTypedReferenceAnalysis(t *testing.T) {
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":      {content: "module example.com/incomplete\n\ngo 1.24\n", mode: 0o644},
		"obsolete.go": {content: "package incomplete\n\nfunc Obsolete() {}\n", mode: 0o644},
		"broken.go":   {content: "package incomplete\n\nfunc Broken(value MissingType) {}\n", mode: 0o644},
	})
	if fixture.analysis.Complete {
		t.Fatal("broken fixture unexpectedly has complete reference analysis")
	}
	_, err := planFixtureDeletion(t, fixture, "obsolete.go")
	if err == nil || !strings.Contains(err.Error(), "complete reference analysis") {
		t.Fatalf("incomplete analysis deletion error=%v", err)
	}
}

func TestDeletionRejectsUnresolvedOpaqueGoBuildDependency(t *testing.T) {
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":      {content: "module example.com/opaque\n\ngo 1.24\n", mode: 0o644},
		"retained.go": {content: "package opaque\n\nfunc Retained() {}\n", mode: 0o644},
		"obsolete.go": {content: "package opaque\n\nfunc Obsolete() {}\n", mode: 0o644},
		"opaque.s":    {content: "// exact opaque build input\n", mode: 0o644},
	})
	if !analysisHasEdgeKind(fixture, "builds_from_opaque") {
		t.Fatal("fixture has no exact opaque build dependency authority")
	}
	_, err := planFixtureDeletion(t, fixture, "obsolete.go")
	if err == nil || !strings.Contains(err.Error(), "unresolved opaque Go build dependency") {
		t.Fatalf("opaque build dependency deletion error=%v", err)
	}
}

func TestDeletionRejectsForceTrackedIgnoredSource(t *testing.T) {
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":      {content: "module example.com/ignored\n\ngo 1.24\n", mode: 0o644},
		"retained.go": {content: "package ignored\n\nfunc Retained() {}\n", mode: 0o644},
		"obsolete.go": {content: "package ignored\n\nfunc Obsolete() {}\n", mode: 0o644},
	})
	if err := os.WriteFile(filepath.Join(fixture.root, ".gitignore"), []byte("obsolete.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, fixture.root, "add", ".gitignore")
	runGit(t, fixture.root, "-c", "user.name=Omnidex Test", "-c", "user.email=test@example.com", "commit", "-m", "ignore tracked source")
	fixture.refresh(t)
	_, err := planFixtureDeletion(t, fixture, "obsolete.go")
	if err == nil || !strings.Contains(err.Error(), "ignored") {
		t.Fatalf("tracked ignored deletion error=%v", err)
	}
}

func TestDeletionStillAllowsExactUnreferencedNonfinalSource(t *testing.T) {
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":      {content: "module example.com/obsolete\n\ngo 1.24\n", mode: 0o644},
		"retained.go": {content: "package obsolete\n\nfunc Retained() {}\n", mode: 0o644},
		"obsolete.go": {content: "package obsolete\n\nfunc Obsolete() {}\n", mode: 0o644},
	})
	stage, err := planFixtureDeletion(t, fixture, "obsolete.go")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
}

func planFixtureDeletion(t *testing.T, fixture *fixture, path string) (*changeapply.StagedChange, error) {
	t.Helper()
	file := fixture.file(t, path)
	removed := make([]string, 0)
	for _, symbol := range fixture.analysis.Symbols {
		if symbol.FileID == file.ID {
			removed = append(removed, symbol.ID)
		}
	}
	return changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, OwnerID: "dependency_closure",
		Desired: []changeapply.DesiredFileState{{
			Path: path, Present: false, RemovedSymbolIDs: removed,
			Source: changeapply.ExactSourceFile{
				FileID: file.ID, SHA256: file.SHA256, Size: file.Size, Mode: file.Mode,
			},
		}},
	})
}

func analysisHasEdgeKind(fixture *fixture, kind string) bool {
	for _, edge := range fixture.analysis.Edges {
		if edge.Kind == kind {
			return true
		}
	}
	return false
}
