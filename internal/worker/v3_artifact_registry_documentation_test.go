package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestArtifactAdapterDocumentationMatchesExecutableRegistries(t *testing.T) {
	t.Parallel()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve registry documentation test source")
	}
	documentPath := filepath.Join(filepath.Dir(sourcePath), "..", "..", "docs", "ARTIFACT_ADAPTERS.md")
	raw, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	document := string(raw)
	adapters := registeredDirectCodingArtifactAdapters()
	sort.Slice(adapters, func(left, right int) bool { return adapters[left].ID < adapters[right].ID })
	adapterRows := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		adapterRows = append(adapterRows, fmt.Sprintf(
			"| `%s` | `%s` |", adapter.ID, adapter.Validation.Kind,
		))
	}
	wantAdapters := strings.Join(append([]string{
		"| Adapter | Executable leaf validation |", "| --- | --- |",
	}, adapterRows...), "\n")
	assertRegistryDocumentationBlock(
		t, document, "ARTIFACT_ADAPTER_REGISTRY", wantAdapters,
	)

	stacks := registeredDirectCodingProjectStacks()
	sort.Slice(stacks, func(left, right int) bool { return stacks[left].ID < stacks[right].ID })
	stackRows := make([]string, 0, len(stacks))
	for _, stack := range stacks {
		defaultSurfaces := "none"
		if len(stack.DefaultSurfaces) != 0 {
			defaultSurfaces = directCodingProjectStackSurfaceSummary(stack.DefaultSurfaces)
		}
		stackRows = append(stackRows, fmt.Sprintf(
			"| `%s` | `%s` | `%s` |", stack.ID,
			directCodingProjectStackSurfaceSummary(stack.SupportedSurfaces), defaultSurfaces,
		))
	}
	wantStacks := strings.Join(append([]string{
		"| Stack | Supported surfaces | Default surfaces |", "| --- | --- | --- |",
	}, stackRows...), "\n")
	assertRegistryDocumentationBlock(t, document, "PROJECT_STACK_REGISTRY", wantStacks)

	profiles := registeredDirectCodingProjectVersionProfiles()
	sort.Slice(profiles, func(left, right int) bool { return profiles[left].ID < profiles[right].ID })
	profileRows := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		manifests := "none"
		if len(profile.ManifestPaths) != 0 {
			manifests = strings.Join(profile.ManifestPaths, ", ")
		}
		stack, err := directCodingProjectStackByID(profile.StackID)
		if err != nil {
			t.Fatal(err)
		}
		defaultValue := "no"
		if stack.DefaultVersionProfileID == profile.ID {
			defaultValue = "yes"
		}
		profileRows = append(profileRows, fmt.Sprintf(
			"| `%s` | `%s` | `%s` | `%s` | `%s` |",
			profile.ID, profile.StackID, profile.SourceDialect, manifests, defaultValue,
		))
	}
	wantProfiles := strings.Join(append([]string{
		"| Version profile | Stack | Source dialect | Manifest evidence | Stack default |",
		"| --- | --- | --- | --- | --- |",
	}, profileRows...), "\n")
	assertRegistryDocumentationBlock(t, document, "PROJECT_VERSION_PROFILE_REGISTRY", wantProfiles)
}

func assertRegistryDocumentationBlock(
	t *testing.T,
	document string,
	name string,
	want string,
) {
	t.Helper()
	begin := "<!-- BEGIN " + name + " -->"
	end := "<!-- END " + name + " -->"
	start := strings.Index(document, begin)
	finish := strings.Index(document, end)
	if start < 0 || finish < 0 || finish <= start {
		t.Fatalf("documentation lacks one ordered %s marker block", name)
	}
	got := strings.TrimSpace(document[start+len(begin) : finish])
	if got != want {
		t.Fatalf("%s documentation differs from executable registry:\nwant:\n%s\n\ngot:\n%s", name, want, got)
	}
}
