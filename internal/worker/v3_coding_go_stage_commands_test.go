package worker

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestGoToolchainVersionUsesNativeCommandAndAcceptsCompatibleVendorBuild(t *testing.T) {
	command := directCodingToolchainVersionCommand("go")
	if !sameExactStrings(command.Argv, []string{"go", "version"}) {
		t.Fatalf("Go version argv=%v; want native go version command", command.Argv)
	}
	if command.Timeout != 15*time.Second || len(command.Environment) != 0 {
		t.Fatalf("Go version command=%+v; want bounded unadorned observation", command)
	}
	profile := directCodingTestGoVersionProfile("1.24.0")
	output := []byte("go version go1.26.3-X:nodwarf5 linux/amd64\n")
	if err := validateDirectCodingToolchainVersion(profile, "go", output); err != nil {
		t.Fatalf("compatible vendor Go compiler was rejected: %v", err)
	}
	observed, err := directCodingGoVersionFromOutput(output)
	if err != nil {
		t.Fatalf("parse observed Go version: %v", err)
	}
	if observed != "v1.26.3" {
		t.Fatalf("observed Go version=%q; want v1.26.3", observed)
	}
}

func TestGoToolchainVersionRejectsOlderMalformedAndPrereleaseCompilers(t *testing.T) {
	profile := directCodingTestGoVersionProfile("1.24.0")
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "older", output: "go version go1.23.9 linux/amd64\n", want: "older than selected profile"},
		{name: "prerelease", output: "go version go1.24rc1 linux/amd64\n", want: "older than selected profile"},
		{name: "wrong command form", output: "go1.26.3", want: "native four-field form"},
		{name: "multiple lines", output: "go version go1.26.3 linux/amd64\nadditional", want: "more than one line"},
		{name: "extra field", output: "go version go1.26.3 vendor linux/amd64", want: "native four-field form"},
		{name: "missing prefix", output: "go version 1.26.3 linux/amd64", want: "go-prefixed release"},
		{name: "malformed release", output: "go version go1.26.3.4 linux/amd64", want: "invalid release"},
		{name: "malformed suffix", output: "go version go1.26.3/evil linux/amd64", want: "invalid release suffix"},
		{name: "malformed platform", output: "go version go1.26.3 linux", want: "invalid platform"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDirectCodingGoToolchainVersion(profile, []byte(test.output))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v; want failure containing %q", err, test.want)
			}
		})
	}
}

func TestGoCommandEnvironmentIsCanonicalOfflineAndExternal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "source")
	cache := filepath.Join(t.TempDir(), "build-cache")
	moduleCache := filepath.Join(t.TempDir(), "module-cache")
	environment, err := directCodingGoCommandEnvironment(root, cache, moduleCache)
	if err != nil {
		t.Fatalf("construct Go environment: %v", err)
	}
	if !sort.StringsAreSorted(environment) {
		t.Fatalf("Go environment is not canonical: %v", environment)
	}
	want := []string{
		"GOCACHE=" + cache,
		"GOENV=off",
		"GOFLAGS=-buildvcs=false -mod=readonly",
		"GOMODCACHE=" + moduleCache,
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOTELEMETRY=off",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
	}
	if !sameExactStrings(environment, want) {
		t.Fatalf("Go environment=%v; want %v", environment, want)
	}
	processEnvironment, err := directCodingVerificationProcessEnvironment(environment)
	if err != nil {
		t.Fatalf("runner rejected registered Go environment: %v", err)
	}
	for _, expected := range want {
		if !containsExactString(processEnvironment, expected) {
			t.Fatalf("process environment omits %q: %v", expected, processEnvironment)
		}
	}
	for _, invalid := range []struct {
		cache       string
		moduleCache string
	}{
		{cache: filepath.Join(root, "cache"), moduleCache: moduleCache},
		{cache: cache, moduleCache: filepath.Join(root, "modules")},
		{cache: cache, moduleCache: cache},
	} {
		if _, err := directCodingGoCommandEnvironment(root, invalid.cache, invalid.moduleCache); err == nil {
			t.Fatalf("accepted invalid cache roots build=%q module=%q", invalid.cache, invalid.moduleCache)
		}
	}
}

func TestGoFormatCheckIsCanonicalAndNonMutating(t *testing.T) {
	root := filepath.Join(t.TempDir(), "source")
	cache := filepath.Join(t.TempDir(), "build-cache")
	moduleCache := filepath.Join(t.TempDir(), "module-cache")
	command, err := directCodingGoFormatCheckCommand(
		root, cache, moduleCache, "runtime.go", "feature001.go", "main.go",
	)
	if err != nil {
		t.Fatalf("construct gofmt check: %v", err)
	}
	wantArgv := []string{
		"gofmt", "-d", "--", "feature001.go", "main.go", "runtime.go",
	}
	if !sameExactStrings(command.Argv, wantArgv) {
		t.Fatalf("gofmt argv=%v; want %v", command.Argv, wantArgv)
	}
	if containsExactString(command.Argv, "-w") {
		t.Fatalf("gofmt check can rewrite source: %v", command.Argv)
	}
	if err := validateDirectCodingGoFormatCheck(directCodingVerificationCommandResult{}); err != nil {
		t.Fatalf("empty gofmt result was rejected: %v", err)
	}
	if err := validateDirectCodingGoFormatCheck(directCodingVerificationCommandResult{
		Stdout: []byte("diff main.go.orig main.go\n"),
	}); err == nil {
		t.Fatal("gofmt diff was accepted")
	}
	if err := validateDirectCodingGoFormatCheck(directCodingVerificationCommandResult{
		Stderr: []byte("gofmt failed"),
	}); err == nil {
		t.Fatal("gofmt stderr was accepted")
	}
	for _, paths := range [][]string{
		nil,
		{"../main.go"},
		{"/tmp/main.go"},
		{"main.go", "main.go"},
		{"README.md"},
	} {
		if _, err := directCodingGoFormatCheckCommand(root, cache, moduleCache, paths...); err == nil {
			t.Fatalf("accepted invalid gofmt paths %v", paths)
		}
	}
}

func TestGoVerificationCommandUsesExactArgumentsAndOfflineEnvironment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "source")
	cache := filepath.Join(t.TempDir(), "build-cache")
	moduleCache := filepath.Join(t.TempDir(), "module-cache")
	command, err := directCodingGoVerificationCommand(
		root, cache, moduleCache, "test", "./...",
	)
	if err != nil {
		t.Fatalf("construct Go verification command: %v", err)
	}
	if !sameExactStrings(command.Argv, []string{"go", "test", "./..."}) {
		t.Fatalf("Go verification argv=%v", command.Argv)
	}
	if command.Timeout != defaultDirectCodingVerificationTimeout ||
		!containsExactString(command.Environment, "GOFLAGS=-buildvcs=false -mod=readonly") ||
		!containsExactString(command.Environment, "GOPROXY=off") ||
		!containsExactString(command.Environment, "GOTOOLCHAIN=local") {
		t.Fatalf("Go verification command lacks code-owned bounds: %+v", command)
	}
}

func directCodingTestGoVersionProfile(version string) directCodingProjectVersionProfile {
	return directCodingProjectVersionProfile{
		ID:      goCommandLineVersionProfileV1,
		StackID: genericGoCommandLineAdapter,
		Components: []directCodingProjectVersionComponent{
			{Name: "go", Version: version},
		},
	}
}
