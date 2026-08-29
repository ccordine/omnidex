package worker

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestVersionProfileSelectionIsAdditiveWithinOneStack(t *testing.T) {
	base := requireDirectCodingVersionProfile(t, goCommandLineVersionProfileV1)
	future := syntheticFutureGoVersionProfile(base)
	if err := future.ValidateDefinition(future); err != nil {
		t.Fatalf("synthetic additive profile is not self-consistent: %v", err)
	}
	manifest := map[string]string{
		"go.mod": "module example.invalid/future\n\ngo 1.25.0\n",
	}

	matched, applicable, err := matchDirectCodingVersionProfiles(
		[]directCodingProjectVersionProfile{base, future}, manifest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if applicable != 2 || len(matched) != 1 || matched[0].ID != future.ID {
		t.Fatalf("applicable=%d matched=%+v", applicable, matched)
	}
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if future.StackID != stack.ID {
		t.Fatalf("future profile changed stacks: profile=%s stack=%s", future.StackID, stack.ID)
	}
	files, err := genericGoCommandLineStaticFiles(future, "future-profile")
	if err != nil {
		t.Fatal(err)
	}
	if source := directCodingTestFileContent(t, files, "go.mod"); !strings.Contains(source, "\ngo 1.25.0\n") {
		t.Fatalf("future profile did not drive static generation:\n%s", source)
	}
}

func TestVersionProfileMatchersExposeClosedTriState(t *testing.T) {
	base := requireDirectCodingVersionProfile(t, goCommandLineVersionProfileV1)
	future := syntheticFutureGoVersionProfile(base)

	for _, testCase := range []struct {
		name      string
		profile   directCodingProjectVersionProfile
		manifests map[string]string
		want      directCodingVersionCompatibility
	}{
		{name: "not applicable", profile: base, manifests: nil, want: directCodingVersionNotApplicable},
		{
			name: "compatible", profile: base,
			manifests: map[string]string{"go.mod": "module example.invalid/base\n\ngo 1.24.0\n"},
			want:      directCodingVersionCompatible,
		},
		{
			name: "unsupported", profile: base,
			manifests: map[string]string{"go.mod": "module example.invalid/future\n\ngo 1.25.0\n"},
			want:      directCodingVersionUnsupported,
		},
		{
			name: "future compatible", profile: future,
			manifests: map[string]string{"go.mod": "module example.invalid/future\n\ngo 1.25.0\n"},
			want:      directCodingVersionCompatible,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := testCase.profile.MatchExisting(testCase.profile, testCase.manifests)
			if err != nil || got != testCase.want {
				t.Fatalf("compatibility=%q want=%q error=%v", got, testCase.want, err)
			}
		})
	}
}

func TestExistingManifestsRejectAmbiguousAndUnknownProfilesBeforeInference(t *testing.T) {
	specification := testProjectStackSpecification(assemblyline.ApplicationSurfaceCommandLine)
	goFiles, err := genericGoCommandLineStaticFiles(
		requireDirectCodingVersionProfile(t, goCommandLineVersionProfileV1), "ambiguous",
	)
	if err != nil {
		t.Fatal(err)
	}
	jsFiles, err := genericJavaScriptCommandLineStaticFiles(
		requireDirectCodingVersionProfile(t, javaScriptCommandLineVersionProfileV1), "ambiguous",
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	resolverCalls := 0
	runtime := typedWorkerRuntime{Execute: testPortableExecutor(func(_, _, _ string) (string, error) {
		calls++
		return "", fmt.Errorf("semantic inference must not run")
	})}
	resolveModel := func() (string, error) {
		resolverCalls++
		return "unused", nil
	}

	_, err = selectDirectCodingProject(runtime, resolveModel, "Build a command-line application.", specification, map[string]string{
		"go.mod":       directCodingTestFileContent(t, goFiles, "go.mod"),
		"package.json": directCodingTestFileContent(t, jsFiles, "package.json"),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "matches 2 registered version profiles") {
		t.Fatalf("ambiguous profile error=%v", err)
	}

	_, err = selectDirectCodingProject(runtime, resolveModel, "Build a command-line application.", specification, map[string]string{
		"go.mod": "module example.invalid/unknown\n\ngo 1.26.0\n",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "no compatible registered version profile") {
		t.Fatalf("unknown profile error=%v", err)
	}
	if calls != 0 || resolverCalls != 0 {
		t.Fatalf("manifest authority triggered model resolvers=%d semantic calls=%d", resolverCalls, calls)
	}
}

func TestVersionProfileCloneOwnsEveryMutableValue(t *testing.T) {
	source := requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1)
	clone := cloneDirectCodingProjectVersionProfile(source)
	clone.ManifestPaths[0] = "changed.json"
	clone.ArtifactVersions[0].Version = "changed"
	clone.Components[0].Version = "changed"
	clone.NPMDependencies["react"] = "0.0.0"
	clone.NPMDevDependencies["typescript"] = "0.0.0"
	clone.NPMLockTemplate[0] ^= 0xff

	fresh := requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1)
	if !reflect.DeepEqual(source.ManifestPaths, fresh.ManifestPaths) ||
		!reflect.DeepEqual(source.ArtifactVersions, fresh.ArtifactVersions) ||
		!reflect.DeepEqual(source.Components, fresh.Components) ||
		!reflect.DeepEqual(source.NPMDependencies, fresh.NPMDependencies) ||
		!reflect.DeepEqual(source.NPMDevDependencies, fresh.NPMDevDependencies) ||
		!reflect.DeepEqual(source.NPMLockTemplate, fresh.NPMLockTemplate) {
		t.Fatal("mutating a returned version profile changed registry-owned authority")
	}
}

func TestVersionConstraintsNormalizeTwoComponentVersionsAndRejectEmptyAlternatives(t *testing.T) {
	for _, testCase := range []struct {
		version    string
		constraint string
		want       bool
	}{
		{version: "1.85", constraint: ">=1.85 <1.86", want: true},
		{version: "1.24", constraint: "1.24.0", want: true},
		{version: "22.9", constraint: "^22.0", want: true},
		{version: "23.0", constraint: "^22.0", want: false},
	} {
		got, err := versionSatisfiesConstraint(testCase.version, testCase.constraint)
		if err != nil || got != testCase.want {
			t.Fatalf("version=%s constraint=%s got=%v want=%v error=%v", testCase.version, testCase.constraint, got, testCase.want, err)
		}
	}
	if !versionAtLeast("1.24", "1.24.0") || !versionAtLeast("1.85", "1.84.9") {
		t.Fatal("two-component versions did not normalize before comparison")
	}
	for _, malformed := range []string{"|| >=1.0", ">=1.0 ||", ">=1.0 || || <2.0"} {
		if _, err := versionSatisfiesConstraint("1.5.0", malformed); err == nil {
			t.Fatalf("accepted malformed constraint %q", malformed)
		}
	}
}

func TestParserQualificationRejectsUnprovenSourceDialect(t *testing.T) {
	adapters := make(map[string]directCodingArtifactAdapter)
	for _, adapter := range registeredDirectCodingArtifactAdapters() {
		adapters[adapter.ID] = adapter
	}
	profile := requireDirectCodingVersionProfile(t, goCommandLineVersionProfileV1)
	if err := validateDirectCodingParserQualifications(adapters, []directCodingProjectVersionProfile{profile}); err != nil {
		t.Fatalf("registered profile lost parser qualification: %v", err)
	}
	profile.SourceDialect = "Go future dialect without parser proof"
	if err := validateDirectCodingParserQualifications(adapters, []directCodingProjectVersionProfile{profile}); err == nil || !strings.Contains(err.Error(), "dialect is not proven") {
		t.Fatalf("unproven dialect error=%v", err)
	}
}

func TestProfileLockAuthorityRejectsFormatDirectPinAndIntegrityDrift(t *testing.T) {
	profile := requireDirectCodingVersionProfile(t, typeScriptBrowserVersionProfileV1)
	for index := range profile.Components {
		if profile.Components[index].Name == "npm_lock" {
			profile.Components[index].Version = "2"
		}
	}
	if err := validateTypeScriptBrowserVersionProfile(profile); err == nil {
		t.Fatal("version profile accepted a lock template with the wrong format")
	}

	validIntegrity := "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64))
	validTemplate := directCodingMinimalLockTemplate("1.0.0", validIntegrity)
	if _, err := materializePinnedNPMLock(
		validTemplate, "fixture", 3, map[string]string{"alpha": "1.0.0"}, nil,
	); err != nil {
		t.Fatalf("valid direct pin was rejected: %v", err)
	}
	if _, err := materializePinnedNPMLock(
		directCodingMinimalLockTemplate("2.0.0", validIntegrity),
		"fixture", 3, map[string]string{"alpha": "1.0.0"}, nil,
	); err == nil {
		t.Fatal("lock accepted a direct package version drift")
	}
	for _, integrity := range []string{
		"sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 63)),
		"sha512-not-base64", "sha256-" + base64.StdEncoding.EncodeToString(make([]byte, 64)),
	} {
		if validSHA512Integrity(integrity) {
			t.Fatalf("accepted invalid direct-package integrity %q", integrity)
		}
		if _, err := materializePinnedNPMLock(
			directCodingMinimalLockTemplate("1.0.0", integrity),
			"fixture", 3, map[string]string{"alpha": "1.0.0"}, nil,
		); err == nil {
			t.Fatalf("lock accepted invalid direct-package integrity %q", integrity)
		}
	}
}

func syntheticFutureGoVersionProfile(
	base directCodingProjectVersionProfile,
) directCodingProjectVersionProfile {
	profile := cloneDirectCodingProjectVersionProfile(base)
	profile.ID = "go_command_line_versions_future_test"
	profile.SourceDialect = "Go 1.25.0"
	for index := range profile.Components {
		switch profile.Components[index].Name {
		case "go":
			profile.Components[index].Version = "1.25.0"
		case "go_manifest":
			profile.Components[index].Version = ">=1.25.0 <1.26.0"
		}
	}
	for index := range profile.ArtifactVersions {
		switch profile.ArtifactVersions[index].AdapterID {
		case "go":
			profile.ArtifactVersions[index].Version = "Go 1.25.0"
		case "go_module":
			profile.ArtifactVersions[index].Version = "Go module 1.25.0"
		}
	}
	return profile
}

func directCodingTestFileContent(
	t *testing.T,
	files []directCodingFileTask,
	path string,
) string {
	t.Helper()
	for _, file := range files {
		if file.Path == path {
			return file.Content
		}
	}
	t.Fatalf("static files omit %s", path)
	return ""
}

func directCodingMinimalLockTemplate(version, integrity string) []byte {
	return []byte(fmt.Sprintf(`{
  "name":"fixture",
  "lockfileVersion":3,
  "packages":{
    "":{"name":"fixture","dependencies":{"alpha":"1.0.0"}},
    "node_modules/alpha":{"version":%q,"integrity":%q}
  }
}`, version, integrity))
}
