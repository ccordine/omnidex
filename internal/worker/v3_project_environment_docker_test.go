package worker

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testProjectEnvironmentImage = "docker:29.5.1-cli@sha256:b40b3737eb3bf588d25bb856d3564dd3f9fdb32ac2fc19ebe85cc58d761692a5"
const testProjectEnvironmentBuiltImageID = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestDirectCodingDockerEnvironmentSpecValidation(t *testing.T) {
	valid := directCodingDockerEnvironmentSpec{
		ID:                "test_environment_v1",
		Dockerfile:        "FROM " + testProjectEnvironmentImage + "\nWORKDIR /workspace\n",
		Programs:          []string{"docker", "touch"},
		WorkspaceReadOnly: true,
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid environment spec: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*directCodingDockerEnvironmentSpec)
		want   string
	}{
		{name: "blank ID", mutate: func(spec *directCodingDockerEnvironmentSpec) { spec.ID = "" }, want: "invalid ID"},
		{name: "mutable base", mutate: func(spec *directCodingDockerEnvironmentSpec) {
			spec.Dockerfile = "FROM docker:29-cli\nWORKDIR /workspace\n"
		}, want: "digest-pinned"},
		{name: "local copy", mutate: func(spec *directCodingDockerEnvironmentSpec) {
			spec.Dockerfile += "COPY project.txt /workspace/project.txt\n"
		}, want: "local build-context input"},
		{name: "duplicate program", mutate: func(spec *directCodingDockerEnvironmentSpec) {
			spec.Programs = []string{"touch", "touch"}
		}, want: "ordered and unique"},
		{name: "unsorted programs", mutate: func(spec *directCodingDockerEnvironmentSpec) {
			spec.Programs = []string{"touch", "docker"}
		}, want: "ordered and unique"},
		{name: "shell program", mutate: func(spec *directCodingDockerEnvironmentSpec) {
			spec.Programs = []string{"sh"}
		}, want: "shell program"},
		{name: "writable workspace", mutate: func(spec *directCodingDockerEnvironmentSpec) {
			spec.WorkspaceReadOnly = false
		}, want: "read-only workspace bind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			candidate.Programs = append([]string(nil), valid.Programs...)
			test.mutate(&candidate)
			if err := candidate.validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestDirectCodingDockerEnvironmentBuildUsesDeterministicInMemoryContext(t *testing.T) {
	root := t.TempDir()
	recorder := &recordingDirectCodingDockerInvoker{}
	environment, err := newDirectCodingDockerProjectEnvironment(
		testDirectCodingDockerEnvironmentSpec(),
		directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
		1234, 5678, recorder.invoke,
	)
	if err != nil {
		t.Fatalf("new environment: %v", err)
	}

	if err := environment.Build(context.Background()); err != nil {
		t.Fatalf("build environment: %v", err)
	}
	if err := environment.Build(context.Background()); err != nil {
		t.Fatalf("repeat build environment: %v", err)
	}
	if len(recorder.calls) != 2 {
		t.Fatalf("docker calls=%d want=2", len(recorder.calls))
	}
	first := recorder.calls[0]
	wantArgs := []string{"build", "--quiet", "--file", "Dockerfile", "-"}
	if !reflect.DeepEqual(first.Args, wantArgs) {
		t.Fatalf("build args=%q want=%q", first.Args, wantArgs)
	}
	if len(first.Stdin) == 0 || !bytes.Equal(first.Stdin, recorder.calls[1].Stdin) {
		t.Fatal("build context is empty or not deterministic")
	}
	if environment.ImageID() != testProjectEnvironmentBuiltImageID {
		t.Fatalf("image ID=%q want=%q", environment.ImageID(), testProjectEnvironmentBuiltImageID)
	}

	reader := tar.NewReader(bytes.NewReader(first.Stdin))
	header, err := reader.Next()
	if err != nil {
		t.Fatalf("read Dockerfile header: %v", err)
	}
	if header.Name != "Dockerfile" || header.Mode != 0o444 || header.Uid != 0 || header.Gid != 0 || !header.ModTime.Equal(time.Unix(0, 0)) {
		t.Fatalf("non-canonical Dockerfile header: %#v", header)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	if string(content) != testDirectCodingDockerEnvironmentSpec().Dockerfile {
		t.Fatalf("Dockerfile=%q", content)
	}
	if _, err := reader.Next(); err != io.EOF {
		t.Fatalf("build context contains more than one entry: %v", err)
	}
}

func TestDirectCodingDockerEnvironmentBuildRejectsMutableOrMalformedImageIdentity(t *testing.T) {
	for _, output := range []string{
		"omnidex-project-environment:latest\n",
		"sha256:ABCDEF\n",
		"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\nextra\n",
		"",
	} {
		t.Run(strings.ReplaceAll(output, "\n", "_"), func(t *testing.T) {
			root := t.TempDir()
			environment, err := newDirectCodingDockerProjectEnvironment(
				testDirectCodingDockerEnvironmentSpec(),
				directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
				1234, 5678,
				func(context.Context, []string, []byte) (v3CommandExecution, error) {
					return v3CommandExecution{ExitCode: 0, Stdout: output}, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := environment.Build(context.Background()); err == nil ||
				!strings.Contains(err.Error(), "invalid immutable image ID") {
				t.Fatalf("build output %q error=%v", output, err)
			}
			if environment.ImageID() != "" {
				t.Fatalf("invalid build retained image ID %q", environment.ImageID())
			}
		})
	}
}

func TestDirectCodingDockerEnvironmentRequiresPersistedAuthorityAfterRebuild(t *testing.T) {
	root := t.TempDir()
	environment, err := newDirectCodingDockerProjectEnvironment(
		testDirectCodingDockerEnvironmentSpec(),
		directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
		1234, 5678, (&recordingDirectCodingDockerInvoker{}).invoke,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := environment.Build(context.Background()); err != nil {
		t.Fatal(err)
	}
	authority, err := environment.Authority()
	if err != nil {
		t.Fatal(err)
	}
	if err := environment.RequireAuthority(authority); err != nil {
		t.Fatalf("require exact authority: %v", err)
	}
	drifted := cloneDirectCodingDockerEnvironmentAuthority(authority)
	drifted.ImageID = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if err := environment.RequireAuthority(drifted); err == nil ||
		!strings.Contains(err.Error(), "differs from persisted authority") {
		t.Fatalf("drifted image authority error=%v", err)
	}
}

func TestDirectCodingDockerEnvironmentRunUsesExactBindAndTypedArgv(t *testing.T) {
	root := t.TempDir()
	recorder := &recordingDirectCodingDockerInvoker{}
	environment, err := newDirectCodingDockerProjectEnvironment(
		testDirectCodingDockerEnvironmentSpec(),
		directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
		1234, 5678, recorder.invoke,
	)
	if err != nil {
		t.Fatalf("new environment: %v", err)
	}
	if err := environment.Build(context.Background()); err != nil {
		t.Fatalf("build environment: %v", err)
	}
	if _, err := environment.Run(context.Background(), directCodingProjectEnvironmentCommand{
		Program: "test", Args: []string{"-f", "visible.txt"}, Timeout: time.Second,
	}); err != nil {
		t.Fatalf("run environment command: %v", err)
	}

	want := []string{
		"run", "--rm", "--read-only",
		"--security-opt", "no-new-privileges=true",
		"--cap-drop", "ALL", "--network", "none",
		"--user", "1234:5678",
		"--mount", "type=bind,src=" + root + ",dst=/workspace,readonly",
		"--tmpfs", "/tmp:rw,nosuid,nodev,noexec",
		"--env", "HOME=/tmp",
		"--workdir", "/workspace", environment.ImageID(),
		"test", "-f", "visible.txt",
	}
	if got := recorder.calls[1].Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("run args=%q want=%q", got, want)
	}
	joined := strings.Join(recorder.calls[1].Args, " ")
	for _, forbidden := range []string{"type=volume", "--volume", " -v ", " sh ", " bash "} {
		if strings.Contains(" "+joined+" ", forbidden) {
			t.Fatalf("run args contain forbidden fallback %q: %s", forbidden, joined)
		}
	}
}

func TestDirectCodingDockerEnvironmentRejectsUnbuiltShellAndHostFallback(t *testing.T) {
	root := t.TempDir()
	recorder := &recordingDirectCodingDockerInvoker{}
	spec := testDirectCodingDockerEnvironmentSpec()
	spec.Programs = []string{"definitely-not-installed-on-host", "sha256sum", "test"}
	environment, err := newDirectCodingDockerProjectEnvironment(
		spec, directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
		1234, 5678, recorder.invoke,
	)
	if err != nil {
		t.Fatalf("new environment: %v", err)
	}
	if _, err := environment.Run(context.Background(), directCodingProjectEnvironmentCommand{
		Program: "test", Args: []string{"-e", "before-build"}, Timeout: time.Second,
	}); err == nil || !strings.Contains(err.Error(), "has not been built") {
		t.Fatalf("unbuilt run error=%v", err)
	}
	if err := environment.Build(context.Background()); err != nil {
		t.Fatalf("build environment: %v", err)
	}
	if _, err := environment.Run(context.Background(), directCodingProjectEnvironmentCommand{
		Program: "definitely-not-installed-on-host", Args: []string{"--version"}, Timeout: time.Second,
	}); err != nil {
		t.Fatalf("typed container-only program unexpectedly resolved on host: %v", err)
	}
	if len(recorder.calls) != 2 || recorder.calls[1].Args[0] != "run" {
		t.Fatalf("container-only command did not go through Docker: %#v", recorder.calls)
	}
	if _, err := environment.Run(context.Background(), directCodingProjectEnvironmentCommand{
		Program: "sh", Args: []string{"-c", "touch escaped"}, Timeout: time.Second,
	}); err == nil || !strings.Contains(err.Error(), "shell program") {
		t.Fatalf("shell error=%v", err)
	}
}

func TestDirectCodingDockerEnvironmentRejectsRootIdentity(t *testing.T) {
	root := t.TempDir()
	for _, identity := range []struct {
		uid uint32
		gid uint32
	}{{uid: 0, gid: 5678}, {uid: 1234, gid: 0}} {
		if _, err := newDirectCodingDockerProjectEnvironment(
			testDirectCodingDockerEnvironmentSpec(),
			directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
			identity.uid, identity.gid, (&recordingDirectCodingDockerInvoker{}).invoke,
		); err == nil || !strings.Contains(err.Error(), "non-root uid:gid") {
			t.Fatalf("identity %d:%d error=%v", identity.uid, identity.gid, err)
		}
	}
}

func TestDirectCodingDockerWorkspaceMappingIsExact(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatalf("create alias: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(alias) })

	tests := []struct {
		name    string
		mapping directCodingDockerWorkspaceMapping
		want    string
	}{
		{name: "relative", mapping: directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: "relative"}, want: "normalized absolute"},
		{name: "comma", mapping: directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root + ",copy"}, want: "Docker mount delimiter"},
		{name: "symlink", mapping: directCodingDockerWorkspaceMapping{RuntimeRoot: alias, HostRoot: alias}, want: "exact directory"},
		{name: "different", mapping: directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: t.TempDir()}, want: "same directory"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.mapping.validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want substring %q", err, test.want)
			}
		})
	}
}

type recordedDirectCodingDockerInvocation struct {
	Args  []string
	Stdin []byte
}

type recordingDirectCodingDockerInvoker struct {
	calls []recordedDirectCodingDockerInvocation
}

func (recorder *recordingDirectCodingDockerInvoker) invoke(
	_ context.Context,
	args []string,
	stdin []byte,
) (v3CommandExecution, error) {
	recorder.calls = append(recorder.calls, recordedDirectCodingDockerInvocation{
		Args: append([]string(nil), args...), Stdin: append([]byte(nil), stdin...),
	})
	if len(args) > 0 && args[0] == "build" {
		return v3CommandExecution{
			ExitCode: 0, Stdout: testProjectEnvironmentBuiltImageID + "\n",
		}, nil
	}
	return v3CommandExecution{ExitCode: 0}, nil
}

func testDirectCodingDockerEnvironmentSpec() directCodingDockerEnvironmentSpec {
	return directCodingDockerEnvironmentSpec{
		ID:                "test_environment_v1",
		Dockerfile:        "FROM " + testProjectEnvironmentImage + "\nWORKDIR /workspace\n",
		Programs:          []string{"sha256sum", "test"},
		WorkspaceReadOnly: true,
	}
}

func testDirectCodingDockerEnvironmentAuthority(
	t *testing.T,
) *directCodingDockerEnvironmentAuthority {
	t.Helper()
	authority, err := newDirectCodingDockerEnvironmentAuthority(
		directCodingPlainTextEnvironmentSpec(), testProjectEnvironmentBuiltImageID,
	)
	if err != nil {
		t.Fatalf("build test project environment authority: %v", err)
	}
	return authority
}
