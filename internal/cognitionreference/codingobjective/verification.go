package codingobjective

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gryph/omnidex/internal/repository/changeapply"
)

const maxVerificationOutputBytes = 1 << 20

func verifyBaseline(ctx context.Context, repository inspectedRepository) (resultErr error) {
	workspace, err := changeapply.NewSnapshotWorkspace(ctx, repository.snapshot)
	if err != nil {
		return fmt.Errorf("create exact baseline workspace: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, workspace.Cleanup()) }()
	outcome, testErr := runGoTests(ctx, workspace.Root())
	if err := workspace.VerifyExact(ctx); err != nil {
		return fmt.Errorf("%w: baseline tests changed their isolated workspace: %v", ErrVerification, err)
	}
	if testErr != nil {
		return fmt.Errorf("%w: execute isolated baseline Go tests: %v", ErrVerification, testErr)
	}
	if outcome.passed {
		return ErrAlreadySatisfied
	}
	return nil
}

func stageAndVerify(
	ctx context.Context,
	repository inspectedRepository,
	candidate string,
) (_ *changeapply.StagedChange, resultErr error) {
	stage, err := changeapply.Plan(ctx, changeapply.Input{
		Snapshot: repository.snapshot, Analysis: repository.analysis,
		Contract: repository.contract,
		Candidates: []changeapply.CandidateDeclaration{{
			SymbolID: repository.symbol.ID, Declaration: candidate,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("stage exact declaration replacement: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			resultErr = errors.Join(resultErr, stage.Cleanup())
		}
	}()
	if err := verifyChangedGoFiles(repository, stage); err != nil {
		return nil, err
	}
	outcome, err := runGoTests(ctx, stage.Workspace())
	if err != nil {
		return nil, fmt.Errorf("%w: execute staged Go verification: %v", ErrVerification, err)
	}
	if !outcome.passed {
		return nil, fmt.Errorf("%w: staged Go verification failed: %s", ErrVerification, outcome.diagnostic)
	}
	if err := stage.VerifyExactWorkspace(ctx); err != nil {
		return nil, fmt.Errorf("%w: staged tests changed exact workspace: %v", ErrVerification, err)
	}
	keep = true
	return stage, nil
}

func verifyChangedGoFiles(
	repository inspectedRepository,
	stage *changeapply.StagedChange,
) error {
	paths := make(map[string]string, len(repository.snapshot.Files))
	for _, file := range repository.snapshot.Files {
		paths[file.ID] = file.Path
	}
	for _, fileID := range stage.ChangedFileIDs() {
		relative, exists := paths[fileID]
		if !exists || filepath.Ext(relative) != ".go" {
			return fmt.Errorf("%w: changed file %q is not one exact Go source", ErrVerification, fileID)
		}
		content, err := os.ReadFile(filepath.Join(stage.Workspace(), filepath.FromSlash(relative)))
		if err != nil {
			return fmt.Errorf("%w: read staged Go source: %v", ErrVerification, err)
		}
		if _, err := parser.ParseFile(token.NewFileSet(), "", content, parser.AllErrors); err != nil {
			return fmt.Errorf("%w: parse complete staged Go source: %v", ErrVerification, err)
		}
		formatted, err := format.Source(content)
		if err != nil {
			return fmt.Errorf("%w: format complete staged Go source: %v", ErrVerification, err)
		}
		if !bytes.Equal(formatted, content) {
			return fmt.Errorf("%w: staged Go source is not canonically formatted", ErrVerification)
		}
	}
	return nil
}

type goTestOutcome struct {
	passed     bool
	diagnostic string
}

func runGoTests(ctx context.Context, root string) (_ goTestOutcome, resultErr error) {
	if err := ctx.Err(); err != nil {
		return goTestOutcome{}, err
	}
	goRoot := filepath.Clean(runtime.GOROOT())
	if goRoot == "." || !filepath.IsAbs(goRoot) {
		return goTestOutcome{}, fmt.Errorf("runtime GOROOT is unavailable")
	}
	goBinary := filepath.Join(goRoot, "bin", "go")
	if info, err := os.Stat(goBinary); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return goTestOutcome{}, fmt.Errorf("runtime Go executable is unavailable")
	}
	privateRoot, err := os.MkdirTemp("/tmp", "omnidex-reference-go-")
	if err != nil {
		return goTestOutcome{}, fmt.Errorf("create private Go verification root: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, removePrivateVerificationRoot(privateRoot))
	}()
	directories := map[string]string{
		"HOME": filepath.Join(privateRoot, "home"), "TMPDIR": filepath.Join(privateRoot, "tmp"),
		"GOCACHE":        filepath.Join(privateRoot, "build-cache"),
		"GOMODCACHE":     filepath.Join(privateRoot, "module-cache"),
		"GOPATH":         filepath.Join(privateRoot, "gopath"),
		"XDG_CACHE_HOME": filepath.Join(privateRoot, "xdg-cache"),
	}
	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return goTestOutcome{}, fmt.Errorf("create private Go verification directory: %w", err)
		}
	}
	command := exec.CommandContext(ctx, goBinary, "test", "-count=1", "./...")
	command.Dir = root
	command.Env = strictGoTestEnvironment(root, goRoot, directories)
	output := &boundedOutput{limit: maxVerificationOutputBytes}
	command.Stdout, command.Stderr = output, output
	err = command.Run()
	if output.overflow {
		return goTestOutcome{}, fmt.Errorf("go test output exceeded %d bytes", maxVerificationOutputBytes)
	}
	if err == nil {
		return goTestOutcome{passed: true, diagnostic: output.Text()}, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return goTestOutcome{}, ctxErr
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return classifyGoTestExit(exitErr.ExitCode(), output.Text())
	}
	return goTestOutcome{}, fmt.Errorf("start or wait for go test: %w", err)
}

func classifyGoTestExit(code int, diagnostic string) (goTestOutcome, error) {
	if code == 1 {
		return goTestOutcome{passed: false, diagnostic: diagnostic}, nil
	}
	return goTestOutcome{}, fmt.Errorf("go test exited with unsupported status %d: %s", code, diagnostic)
}

func strictGoTestEnvironment(root, goRoot string, directories map[string]string) []string {
	return []string{
		"PATH=" + filepath.Join(goRoot, "bin"), "GOROOT=" + goRoot,
		"HOME=" + directories["HOME"], "TMPDIR=" + directories["TMPDIR"],
		"GOCACHE=" + directories["GOCACHE"], "GOMODCACHE=" + directories["GOMODCACHE"],
		"GOPATH=" + directories["GOPATH"], "XDG_CACHE_HOME=" + directories["XDG_CACHE_HOME"],
		"PWD=" + root, "LANG=C", "LC_ALL=C", "GOENV=off", "GOWORK=off",
		"GOFLAGS=-mod=readonly", "GOVCS=off", "GOPROXY=off", "GOSUMDB=off",
		"GOTOOLCHAIN=local", "CGO_ENABLED=0", "GO111MODULE=on",
	}
}

func removePrivateVerificationRoot(root string) error {
	if root == "" || filepath.Dir(root) != "/tmp" || !strings.HasPrefix(filepath.Base(root), "omnidex-reference-go-") {
		return fmt.Errorf("refuse to remove invalid private Go verification root %q", root)
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("remove private Go verification root: %w", err)
	}
	return nil
}

type boundedOutput struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (output *boundedOutput) Write(value []byte) (int, error) {
	if output.buffer.Len()+len(value) > output.limit {
		remaining := output.limit - output.buffer.Len()
		if remaining > 0 {
			_, _ = output.buffer.Write(value[:remaining])
		}
		output.overflow = true
		return len(value), nil
	}
	return output.buffer.Write(value)
}

func (output *boundedOutput) Text() string {
	value := strings.TrimSpace(output.buffer.String())
	if output.overflow {
		value += "...[truncated]"
	}
	return value
}
