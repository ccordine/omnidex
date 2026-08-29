package worker

import (
	"fmt"
	"strings"
	"testing"
)

func TestRegisteredVersionProfilesUseExactQualifiedRuntimeProbes(t *testing.T) {
	outputs := map[string]string{
		versionProbeKey("node", "--version"):                                    "v25.2.1",
		versionProbeKey("npm", "--version"):                                     "11.6.2",
		versionProbeKey("go", "version"):                                        "go version go1.26.3 linux/amd64",
		versionProbeKey("rustc", "--version"):                                   "rustc 1.95.0 (abcdef012 2026-08-01)",
		versionProbeKey("cargo", "--version"):                                   "cargo 1.95.0 (abcdef012 2026-08-01)",
		versionProbeKey("javac", "--release", "21", "-version"):                 "javac 26.0.1",
		versionProbeKey("java", "-version"):                                     "openjdk version \"26.0.1\" 2026-04-01\nOpenJDK Runtime Environment (build 26.0.1+1)\nOpenJDK 64-Bit Server VM (build 26.0.1+1, mixed mode)",
		versionProbeKey("jar", "--version"):                                     "jar 26.0.1",
		versionProbeKey("docker", "version", "--format", "{{.Server.Version}}"): directCodingDockerEngineVersion,
		versionProbeKey("docker", "compose", "version", "--short"):              directCodingDockerComposeVersion,
	}
	seen := make(map[string]int)
	probe := func(program string, args ...string) (string, error) {
		key := versionProbeKey(program, args...)
		output, exists := outputs[key]
		if !exists {
			return "", fmt.Errorf("unexpected version probe %q", key)
		}
		seen[key]++
		return output, nil
	}
	for _, profile := range registeredDirectCodingProjectVersionProfiles() {
		if err := validateDirectCodingVersionProfileRuntime(profile, probe); err != nil {
			t.Fatalf("profile %s runtime qualification: %v", profile.ID, err)
		}
	}
	for key := range outputs {
		if seen[key] == 0 {
			t.Fatalf("registered profiles never executed exact probe %q", key)
		}
	}
}

func TestPHPProfilesRejectDifferentDockerComposeRuntime(t *testing.T) {
	t.Parallel()
	probe := func(program string, args ...string) (string, error) {
		switch versionProbeKey(program, args...) {
		case versionProbeKey("docker", "version", "--format", "{{.Server.Version}}"):
			return directCodingDockerEngineVersion, nil
		case versionProbeKey("docker", "compose", "version", "--short"):
			return "5.1.3", nil
		default:
			return "", fmt.Errorf("unexpected version probe")
		}
	}
	for _, profileID := range []string{phpServiceVersionProfileV1, laravelVersionProfileV1} {
		profile := requireDirectCodingVersionProfile(t, profileID)
		if err := validateDirectCodingVersionProfileRuntime(profile, probe); err == nil ||
			!strings.Contains(err.Error(), directCodingDockerComposeVersion) {
			t.Fatalf("profile %s accepted a different Compose runtime: %v", profileID, err)
		}
	}
}

func TestPHPProfilesRejectDifferentDockerEngineRuntime(t *testing.T) {
	t.Parallel()
	probe := func(program string, args ...string) (string, error) {
		switch versionProbeKey(program, args...) {
		case versionProbeKey("docker", "version", "--format", "{{.Server.Version}}"):
			return "29.5.0", nil
		case versionProbeKey("docker", "compose", "version", "--short"):
			return directCodingDockerComposeVersion, nil
		default:
			return "", fmt.Errorf("unexpected version probe")
		}
	}
	for _, profileID := range []string{phpServiceVersionProfileV1, laravelVersionProfileV1} {
		profile := requireDirectCodingVersionProfile(t, profileID)
		if err := validateDirectCodingVersionProfileRuntime(profile, probe); err == nil ||
			!strings.Contains(err.Error(), directCodingDockerEngineVersion) {
			t.Fatalf("profile %s accepted a different Docker Engine runtime: %v", profileID, err)
		}
	}
}

func TestRuntimeVersionGrammarsAcceptOnlyExactToolOutput(t *testing.T) {
	for _, testCase := range []struct {
		program string
		output  string
		want    string
	}{
		{program: "node", output: "v25.2.1", want: "25.2.1"},
		{program: "npm", output: "11.6.2", want: "11.6.2"},
		{program: "go", output: "go version go1.26.3 linux/amd64", want: "1.26.3"},
		{program: "rustc", output: "rustc 1.95.0 (abcdef012 2026-08-01)", want: "1.95.0"},
		{program: "cargo", output: "cargo 1.95.0 (abcdef012 2026-08-01)", want: "1.95.0"},
		{program: "javac", output: "javac 26.0.1", want: "26.0.1"},
		{
			program: "java",
			output:  "openjdk version \"26.0.1\" 2026-04-01\nOpenJDK Runtime Environment (build 26.0.1+1)\nOpenJDK 64-Bit Server VM (build 26.0.1+1, mixed mode)",
			want:    "26.0.1",
		},
		{program: "jar", output: "jar 26.0.1", want: "26.0.1"},
		{program: "docker", output: directCodingDockerEngineVersion, want: directCodingDockerEngineVersion},
	} {
		t.Run(testCase.program, func(t *testing.T) {
			got, err := directCodingRuntimeCommandVersion(
				func(string, ...string) (string, error) { return testCase.output, nil },
				testCase.program,
			)
			if err != nil || got != testCase.want {
				t.Fatalf("version=%q want=%q error=%v", got, testCase.want, err)
			}
		})
	}

	for _, testCase := range []struct {
		program string
		output  string
	}{
		{program: "node", output: "v25.2.1 extra"},
		{program: "npm", output: "npm 11.6.2"},
		{program: "go", output: "go1.26.3"},
		{program: "rustc", output: "rustc 1.95.0"},
		{program: "cargo", output: "cargo 1.95.0\nverbose"},
		{program: "javac", output: "javac version 26.0.1"},
		{program: "java", output: "openjdk version \"26.0.1\""},
		{program: "jar", output: "jar tool 26.0.1"},
		{program: "docker", output: "Docker version 29.1.3"},
	} {
		t.Run("reject "+testCase.program, func(t *testing.T) {
			_, err := directCodingRuntimeCommandVersion(
				func(string, ...string) (string, error) { return testCase.output, nil },
				testCase.program,
			)
			if err == nil {
				t.Fatalf("accepted non-exact %s output %q", testCase.program, testCase.output)
			}
		})
	}
}

func TestVersionProbeCommandsAreAnExactAllowlist(t *testing.T) {
	valid := []struct {
		program string
		args    []string
	}{
		{program: "node", args: []string{"--version"}},
		{program: "npm", args: []string{"--version"}},
		{program: "go", args: []string{"version"}},
		{program: "rustc", args: []string{"--version"}},
		{program: "cargo", args: []string{"--version"}},
		{program: "javac", args: []string{"--release", "21", "-version"}},
		{program: "java", args: []string{"-version"}},
		{program: "jar", args: []string{"--version"}},
		{program: "docker", args: []string{"version", "--format", "{{.Server.Version}}"}},
		{program: "docker", args: []string{"compose", "version", "--short"}},
	}
	for _, command := range valid {
		if err := validateV3Command(command.program, command.args); err != nil {
			t.Fatalf("exact probe %s %v rejected: %v", command.program, command.args, err)
		}
	}

	invalid := []struct {
		program string
		args    []string
	}{
		{program: "node", args: []string{"--version", "extra"}},
		{program: "npm", args: []string{"--version", "--json"}},
		{program: "go", args: []string{"version", "extra"}},
		{program: "rustc", args: []string{"--version", "--verbose"}},
		{program: "cargo", args: []string{"--version", "--verbose"}},
		{program: "javac", args: []string{"--release", "21", "-version", "extra"}},
		{program: "java", args: []string{"-version", "extra"}},
		{program: "jar", args: []string{"--version", "extra"}},
		{program: "docker", args: []string{"version", "--format", "{{.Server.Version}}", "extra"}},
		{program: "docker", args: []string{"compose", "version", "--short", "extra"}},
	}
	for _, command := range invalid {
		if err := validateV3Command(command.program, command.args); err == nil {
			t.Fatalf("accepted non-exact probe %s %v", command.program, command.args)
		}
	}
}

func versionProbeKey(program string, args ...string) string {
	return strings.Join(append([]string{program}, args...), "\x00")
}
