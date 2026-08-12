package version_test

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitiongauntlet"
)

func TestLabyrinthFirstRunPinsOneSealedRetrieveDiagnostic(t *testing.T) {
	root := runbookRepositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "docs", "LABYRINTH_FIRST_RUN.md"))
	if err != nil {
		t.Fatal(err)
	}
	document := string(raw)
	for _, required := range []string{
		"Status: blocked until both hard gates pass.",
		"num_ctx=32768", "not the architecture maximum",
		"checked migration manifest", "verified semantic replay binding",
		"before accepting each child receipt", "all 9 replay bindings",
		"exactly 9 variants", "non-promotional",
		"`raw_shell` is benchmark-only", "`oracle_evidence_packet` is oracle-contaminated",
		`GAUNTLET="$RELEASE_ROOT/bin/cognition-gauntlet"`,
		`test ! -e "$RELEASE_ROOT"`,
		`tar -xzf omnidex-v0.5.0-linux-amd64.tar.gz -C "$RELEASE_ROOT"`,
		`prepare-matrix --request "$REQUEST_PATH" --config "$CONFIG_PATH"`,
		`matrix --config "$CONFIG_PATH"`,
		`verify-matrix --config "$CONFIG_PATH"`,
		"migrations/SHA256SUMS", "sha256sum -c SHA256SUMS",
	} {
		if !strings.Contains(document, required) {
			t.Fatalf("first-run runbook lacks %q", required)
		}
	}
	for _, forbidden := range []string{"go run ", "go build ", "./cmd/cognition-gauntlet", "--enable-promotion"} {
		if strings.Contains(document, forbidden) {
			t.Fatalf("first-run runbook exposes forbidden source or promotion command %q", forbidden)
		}
	}

	request := decodeRunbookMatrixRequest(t, document)
	wantBudget := cognitiongauntlet.InitialMicrogauntletsV2()[0].Budget
	if request.Schema != cognitiongauntlet.OfflineMatrixRequestSchemaV2 ||
		request.Plan.Policy != cognitiongauntlet.CompetenceSuccessSuperiority ||
		len(request.Plan.Suites) != 1 || request.Plan.Suites[0] != cognitiongauntlet.SuiteRetrieve ||
		len(request.Plan.Seeds) != 1 || request.Plan.Seeds[0] != 11_001 ||
		request.Plan.Repetitions != 1 || request.Plan.Surface != cognitiongauntlet.SurfaceFilesystem ||
		request.Budget != wantBudget || request.Brain.Model != "qwen3.5:9b-q4_K_M" ||
		request.Brain.NativeContextLimit != 32_768 {
		t.Fatalf("first-run request differs from the exact Retrieve diagnostic: %+v", request)
	}
	for _, directory := range []string{request.PublicOutputDirectory, request.PrivateOutputDirectory} {
		if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
			t.Fatalf("first-run output directory is not strict absolute: %q", directory)
		}
	}
	if request.PublicOutputDirectory == request.PrivateOutputDirectory ||
		strings.HasPrefix(request.PrivateOutputDirectory, request.PublicOutputDirectory+string(filepath.Separator)) ||
		strings.HasPrefix(request.PublicOutputDirectory, request.PrivateOutputDirectory+string(filepath.Separator)) {
		t.Fatal("first-run public and private directories are not disjoint siblings")
	}
}

func TestReleaseBuilderPackagesExactLabyrinthFirstRunRunbook(t *testing.T) {
	root := runbookRepositoryRoot(t)
	script := filepath.Join(root, "scripts", "build-release.sh")
	scriptRaw, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	scriptSource := string(scriptRaw)
	packageCall := `package_release_operator_runbook "$target_source" "$target_dir"`
	packageAt := strings.Index(scriptSource, packageCall)
	archiveAt := strings.Index(scriptSource, `archive_target "$target_dir" "$target_name" "$goos"`)
	if strings.Count(scriptSource, packageCall) != 1 || packageAt < 0 || archiveAt < packageAt {
		t.Fatal("release target does not package the operator runbook")
	}

	source := filepath.Join(t.TempDir(), "source")
	target := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(filepath.Join(source, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(root, "docs", "LABYRINTH_FIRST_RUN.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "docs", "LABYRINTH_FIRST_RUN.md"), want, 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := runRunbookPackager(script, source, target)
	if err != nil {
		t.Fatalf("package first-run runbook: %v: %s", err, output)
	}
	got, err := os.ReadFile(filepath.Join(target, "LABYRINTH_FIRST_RUN.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("packaged first-run runbook differs from its sealed source")
	}

	missing := filepath.Join(t.TempDir(), "missing-source")
	if err := os.Mkdir(missing, 0o755); err != nil {
		t.Fatal(err)
	}
	output, err = runRunbookPackager(script, missing, filepath.Join(t.TempDir(), "target"))
	if err == nil || !strings.Contains(output, "Labyrinth first-run runbook") {
		t.Fatalf("missing first-run runbook error=%v output=%q", err, output)
	}
}

func decodeRunbookMatrixRequest(t *testing.T, document string) cognitiongauntlet.OfflineMatrixRequest {
	t.Helper()
	const fence = "```json\n"
	start := strings.Index(document, fence)
	if start < 0 || strings.Count(document, fence) != 1 {
		t.Fatal("first-run runbook must contain exactly one JSON request block")
	}
	content := document[start+len(fence):]
	end := strings.Index(content, "\n```")
	if end < 0 {
		t.Fatal("first-run JSON request block is unterminated")
	}
	decoder := json.NewDecoder(strings.NewReader(content[:end]))
	decoder.DisallowUnknownFields()
	var request cognitiongauntlet.OfflineMatrixRequest
	if err := decoder.Decode(&request); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("first-run JSON request has trailing content: %v", err)
	}
	return request
}

func runRunbookPackager(script, source, target string) (string, error) {
	command := exec.Command(
		"bash", "-c", `source "$1"; package_release_operator_runbook "$2" "$3"`,
		"release-runbook-test", script, source, target,
	)
	output, err := command.CombinedOutput()
	return string(output), err
}

func runbookRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
