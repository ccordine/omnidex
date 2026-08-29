package worker

import (
	"strings"
	"testing"
)

func TestVersionProfileDefinitionsRejectImpreciseOrDivergentAuthority(t *testing.T) {
	testCases := []struct {
		name      string
		profileID string
		mutate    func(*directCodingProjectVersionProfile)
		want      string
	}{
		{
			name: "Go generated version must be exact", profileID: goCommandLineVersionProfileV1,
			mutate: func(profile *directCodingProjectVersionProfile) {
				setDirectCodingTestVersionComponent(profile, "go", ">=1.24.0")
			},
			want: "requires one exact semantic version",
		},
		{
			name: "Rust manifest and runtime must agree", profileID: rustCommandLineVersionProfileV1,
			mutate: func(profile *directCodingProjectVersionProfile) {
				setDirectCodingTestVersionComponent(profile, "rust_manifest", "1.86.0")
			},
			want: "Rust manifest and runtime versions differ",
		},
		{
			name: "Java release must be integral", profileID: javaCommandLineVersionProfileV1,
			mutate: func(profile *directCodingProjectVersionProfile) {
				setDirectCodingTestVersionComponent(profile, "java_release", "21.0")
			},
			want: "invalid Java release",
		},
		{
			name: "PHP image must retain digest", profileID: phpServiceVersionProfileV1,
			mutate: func(profile *directCodingProjectVersionProfile) {
				setDirectCodingTestVersionComponent(profile, "nginx_image", "nginx:latest")
			},
			want: "digest-pinned container image",
		},
		{
			name: "PHP Docker Engine must be exact", profileID: phpServiceVersionProfileV1,
			mutate: func(profile *directCodingProjectVersionProfile) {
				setDirectCodingTestVersionComponent(profile, "docker_engine", ">=29.5.1")
			},
			want: "requires one exact semantic version",
		},
		{
			name: "Laravel Docker Compose must be exact", profileID: laravelVersionProfileV1,
			mutate: func(profile *directCodingProjectVersionProfile) {
				setDirectCodingTestVersionComponent(profile, "docker_compose", "^5.1.4")
			},
			want: "requires one exact semantic version",
		},
		{
			name: "TypeScript npm packages must be exact", profileID: typeScriptBrowserVersionProfileV1,
			mutate: func(profile *directCodingProjectVersionProfile) {
				setDirectCodingTestVersionComponent(profile, "typescript", "^5.9.3")
				profile.NPMDevDependencies["typescript"] = "^5.9.3"
			},
			want: "requires one exact semantic version",
		},
		{
			name: "PHP npm packages must be exact", profileID: phpServiceVersionProfileV1,
			mutate: func(profile *directCodingProjectVersionProfile) {
				setDirectCodingTestVersionComponent(profile, "tailwindcss", "^4.1.12")
				profile.NPMDevDependencies["tailwindcss"] = "^4.1.12"
			},
			want: "requires one exact semantic version",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			profile := requireDirectCodingVersionProfile(t, testCase.profileID)
			testCase.mutate(&profile)
			err := profile.ValidateDefinition(profile)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("definition error=%v want substring %q", err, testCase.want)
			}
		})
	}
}

func setDirectCodingTestVersionComponent(
	profile *directCodingProjectVersionProfile,
	name string,
	value string,
) {
	for index := range profile.Components {
		if profile.Components[index].Name == name {
			profile.Components[index].Version = value
			return
		}
	}
	panic("test profile lacks component " + name)
}
