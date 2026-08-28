package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestExplicitPlainTextArtifactPathsAreMechanicalAdapterFacts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		authority string
		want      []string
	}{
		{name: "one", authority: "Create proof.txt containing one line.", want: []string{"proof.txt"}},
		{name: "quoted spaces", authority: `Create "Docs/Proof Record.TXT".`, want: []string{"Docs/Proof Record.TXT"}},
		{name: "deduplicated", authority: "Create proof.txt and verify proof.txt.", want: []string{"proof.txt"}},
		{name: "plural sorted", authority: "Create z.txt and a.txt.", want: []string{"a.txt", "z.txt"}},
		{name: "semantic dotted name", authority: "Use Node.js.", want: []string{}},
		{name: "other adapter", authority: "Create proof.go.", want: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths, err := explicitPlainTextArtifactPaths(test.authority)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(paths, "\x00") != strings.Join(test.want, "\x00") {
				t.Fatalf("paths=%q want=%q", paths, test.want)
			}
		})
	}
}

func TestSinglePlainTextTargetIsAbsentFromRenderedSourceEnvelope(t *testing.T) {
	t.Parallel()
	const artifactPath = "PROOF.TXT"
	const authority = "Create PROOF.TXT containing exactly one line: host proof."
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{artifactPath})
	if err != nil {
		t.Fatal(err)
	}
	redacted, identities, err := assemblyline.RedactArtifactIdentities(authority, provenance)
	if err != nil {
		t.Fatal(err)
	}
	if !artifactIdentityBoundToPath(identities, artifactPath) || strings.Contains(redacted, artifactPath) {
		t.Fatalf("redacted=%q identities=%+v", redacted, identities)
	}
	task, err := assemblyline.FreezePlainTextArtifactTask(authority, redacted)
	if err != nil {
		t.Fatal(err)
	}
	target := assemblyline.TargetTree{
		StackID:          assemblyline.PlainTextArtifactStackID,
		VersionProfileID: assemblyline.PlainTextArtifactProfileID,
		Paths:            []string{artifactPath},
	}
	coverage, err := assemblyline.NewPlainTextArtifactCoverage(task, target)
	if err != nil {
		t.Fatal(err)
	}
	blueprint, err := assemblyline.CompilePlainTextArtifactBlueprint(task, target, coverage)
	if err != nil {
		t.Fatal(err)
	}
	block := blueprint.Documents[0].Blocks[0]
	job, err := assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
		Language: assemblyline.TextFragmentLanguage, Dialect: assemblyline.TextFragmentDialect,
		Signature: block.Signature, Behavior: block.Contract,
		Capabilities: []string{}, PermittedSymbols: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, artifactPath) || !strings.Contains(prompt, "ARTIFACT_1") {
		t.Fatalf("source envelope leaked or lost target identity: %s", prompt)
	}
	if err := validatePlainTextPathBlindValue("Write proof.txt exactly.\n"); err == nil {
		t.Fatal("bare adapter-recognizable path escaped path-blind validation")
	}
}

func TestSingleArtifactCreationTargetUsesFullWorkspaceAuthority(t *testing.T) {
	root := singleArtifactGitFixture(t)
	snapshot, err := repositoryfacts.BuildGitSnapshot(
		t.Context(), root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	source, err := workspacefacts.FromRepositorySnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	ownerID := singleArtifactMutationOwnerID("create proof", snapshot.ID)
	selected, _, err := directCodingArtifactAdapterForPath("proof.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name string
		path string
		want string
	}{
		{name: "new plain text", path: "proof.txt"},
		{name: "existing", path: "README.txt", want: "safe absent"},
		{name: "protected", path: ".git/proof.txt", want: "protected authority"},
		{name: "unsupported", path: "proof.md", want: "registered adapter"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateSingleArtifactCreationTarget(
				t.Context(), source, ownerID,
				assemblyline.TargetTree{
					StackID:          assemblyline.PlainTextArtifactStackID,
					VersionProfileID: assemblyline.PlainTextArtifactProfileID,
					Paths:            []string{testCase.path},
				}, selected,
			)
			if testCase.want == "" && err != nil {
				t.Fatalf("valid target: %v", err)
			}
			if testCase.want != "" && (err == nil || !strings.Contains(err.Error(), testCase.want)) {
				t.Fatalf("target error=%v want %q", err, testCase.want)
			}
		})
	}
}

func TestSinglePlainTextRepositoryPostRequiresExactHostFile(t *testing.T) {
	root := singleArtifactGitFixture(t)
	before, err := repositoryfacts.BuildGitSnapshot(
		t.Context(), root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("host exact\n")
	if err := os.WriteFile(filepath.Join(root, "proof.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := repositoryfacts.BuildGitSnapshot(
		t.Context(), root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSinglePlainTextRepositoryPost(
		before, after, "proof.txt", content,
	); err != nil {
		t.Fatal(err)
	}
	if err := validateSinglePlainTextRepositoryPost(
		before, after, "proof.txt", []byte("different\n"),
	); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("content mismatch error=%v", err)
	}
}

func TestPlainTextWorkspaceVerificationCommandIsDockerEnvironmentOnly(t *testing.T) {
	command := plainTextWorkspaceVerificationCommand(
		"proof.txt", testDirectCodingDockerEnvironmentAuthority(t),
	)
	if err := validatePlainTextWorkspaceVerificationCommand(command); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*testCommand){
		func(value *testCommand) { value.Name = "go" },
		func(value *testCommand) { value.Args = []string{"proof.txt", "other.txt"} },
		func(value *testCommand) { value.Timeout = 0 },
		func(value *testCommand) { value.Purpose = verificationBuild },
		func(value *testCommand) { value.Environment = nil },
	} {
		candidate := command
		candidate.Args = append([]string(nil), command.Args...)
		mutate(&candidate)
		if err := validatePlainTextWorkspaceVerificationCommand(candidate); err == nil {
			t.Fatalf("accepted invalid plain-text command: %+v", candidate)
		}
	}
	if command.Timeout != 30*time.Second {
		t.Fatalf("plain-text command timeout=%s", command.Timeout)
	}
}

func singleArtifactGitFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repositorySandboxGit(t, root, "init")
	repositorySandboxGit(t, root, "config", "user.email", "test@example.com")
	repositorySandboxGit(t, root, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "README.txt"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repositorySandboxGit(t, root, "add", "README.txt")
	repositorySandboxGit(t, root, "commit", "-m", "fixture")
	return root
}
