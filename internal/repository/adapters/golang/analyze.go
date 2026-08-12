package golang

import (
	"context"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"golang.org/x/tools/go/packages"
)

const (
	AdapterName    = "go"
	AdapterVersion = "go-packages-v1"
)

var adapterIdentity = repositoryfacts.AdapterIdentity{Name: AdapterName, Version: AdapterVersion}

func Analyze(ctx context.Context, snapshot repositoryfacts.Snapshot) (repositoryfacts.Analysis, error) {
	if ctx == nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("Go repository analysis requires a context")
	}
	if err := snapshot.Validate(); err != nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("Go repository analysis snapshot: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("Go repository analysis: %w", err)
	}
	state, err := newAnalysisState(snapshot)
	if err != nil {
		return repositoryfacts.Analysis{}, err
	}
	fileSet := token.NewFileSet()
	loaded, err := packages.Load(&packages.Config{
		Context: ctx,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
		Dir: snapshot.Root, Fset: fileSet, Tests: true,
		Env: goAnalysisEnvironment(os.Environ(), snapshot.Root, exactGoWork(snapshot)),
	}, "./...")
	if err != nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("load Go repository packages: %w", err)
	}
	all := flattenPackages(loaded)
	for _, pkg := range all {
		if err := state.collectPackageFacts(fileSet, pkg); err != nil {
			return repositoryfacts.Analysis{}, err
		}
	}
	for _, pkg := range all {
		if err := state.collectPackageRelations(fileSet, pkg); err != nil {
			return repositoryfacts.Analysis{}, err
		}
	}
	if err := state.validateCompleteFileCoverage(); err != nil {
		return repositoryfacts.Analysis{}, err
	}
	state.collectDiagnostics(all)
	analysis := state.analysis()
	if err := repositoryfacts.FinalizeAnalysis(&analysis); err != nil {
		return repositoryfacts.Analysis{}, err
	}
	if err := analysis.Validate(snapshot); err != nil {
		return repositoryfacts.Analysis{}, err
	}
	return analysis, nil
}

type analysisState struct {
	snapshot            repositoryfacts.Snapshot
	filesByAbsolute     map[string]repositoryfacts.File
	symbols             map[string]repositoryfacts.Symbol
	symbolIDByQualified map[string]string
	symbolKindByID      map[string]string
	artifacts           map[string]repositoryfacts.Artifact
	packageArtifactIDs  map[string]string
	edges               map[string]repositoryfacts.Edge
	diagnostics         map[string]repositoryfacts.AnalysisDiagnostic
	relevantPackages    map[string]struct{}
	coveredFileIDs      map[string]struct{}
}

func newAnalysisState(snapshot repositoryfacts.Snapshot) (*analysisState, error) {
	files := make(map[string]repositoryfacts.File)
	goFiles := 0
	for _, file := range snapshot.Files {
		if file.Kind != repositoryfacts.EntryRegular || file.Language != "go" {
			continue
		}
		absolute := filepath.Clean(filepath.Join(snapshot.Root, filepath.FromSlash(file.Path)))
		files[absolute] = file
		goFiles++
	}
	if goFiles == 0 {
		return nil, fmt.Errorf("Go repository analysis requires at least one indexed Go source file")
	}
	return &analysisState{
		snapshot: snapshot, filesByAbsolute: files,
		symbols:             make(map[string]repositoryfacts.Symbol),
		symbolIDByQualified: make(map[string]string), symbolKindByID: make(map[string]string),
		artifacts: make(map[string]repositoryfacts.Artifact), packageArtifactIDs: make(map[string]string),
		edges:            make(map[string]repositoryfacts.Edge),
		diagnostics:      make(map[string]repositoryfacts.AnalysisDiagnostic),
		relevantPackages: make(map[string]struct{}),
		coveredFileIDs:   make(map[string]struct{}),
	}, nil
}

func (state *analysisState) analysis() repositoryfacts.Analysis {
	analysis := repositoryfacts.Analysis{
		Schema: repositoryfacts.AnalysisSchemaV1, SnapshotID: state.snapshot.ID,
		Adapter: adapterIdentity, Complete: true, GeneratedAt: time.Now().UTC(),
		Symbols:     make([]repositoryfacts.Symbol, 0, len(state.symbols)),
		Artifacts:   make([]repositoryfacts.Artifact, 0, len(state.artifacts)),
		Edges:       make([]repositoryfacts.Edge, 0, len(state.edges)),
		Diagnostics: make([]repositoryfacts.AnalysisDiagnostic, 0, len(state.diagnostics)),
	}
	for _, symbol := range state.symbols {
		analysis.Symbols = append(analysis.Symbols, symbol)
	}
	for _, artifact := range state.artifacts {
		analysis.Artifacts = append(analysis.Artifacts, artifact)
	}
	for _, edge := range state.edges {
		analysis.Edges = append(analysis.Edges, edge)
	}
	for _, diagnostic := range state.diagnostics {
		analysis.Diagnostics = append(analysis.Diagnostics, diagnostic)
		if diagnostic.Severity == "error" {
			analysis.Complete = false
		}
	}
	return analysis
}

func (state *analysisState) indexedFile(path string) (repositoryfacts.File, bool) {
	file, ok := state.filesByAbsolute[filepath.Clean(path)]
	return file, ok
}

func (state *analysisState) validateCompleteFileCoverage() error {
	missing := make([]string, 0)
	for _, file := range state.filesByAbsolute {
		if _, covered := state.coveredFileIDs[file.ID]; !covered {
			missing = append(missing, file.ID)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf(
		"Go repository analysis did not cover %d indexed Go files; first missing authority is %q",
		len(missing), missing[0],
	)
}

func flattenPackages(roots []*packages.Package) []*packages.Package {
	seen := make(map[string]*packages.Package)
	packages.Visit(roots, nil, func(pkg *packages.Package) {
		if pkg != nil {
			seen[pkg.ID] = pkg
		}
	})
	items := make([]*packages.Package, 0, len(seen))
	for _, pkg := range seen {
		items = append(items, pkg)
	}
	sort.Slice(items, func(left, right int) bool { return items[left].ID < items[right].ID })
	return items
}

func exactGoWork(snapshot repositoryfacts.Snapshot) string {
	for _, file := range snapshot.Files {
		if file.Path == "go.work" && file.Kind == repositoryfacts.EntryRegular {
			return filepath.Join(snapshot.Root, "go.work")
		}
	}
	return "off"
}

func goAnalysisEnvironment(current []string, root, goWork string) []string {
	out := make([]string, 0, len(current)+9)
	for _, item := range current {
		key, _, _ := strings.Cut(item, "=")
		switch key {
		case "GO111MODULE", "GOENV", "GOFLAGS", "GOWORK", "GOPROXY", "GOSUMDB", "GOTOOLCHAIN", "GOVCS", "PWD":
			continue
		default:
			out = append(out, item)
		}
	}
	return append(
		out, "GO111MODULE=on", "GOENV=off", "GOFLAGS=-mod=readonly", "GOWORK="+goWork,
		"GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOVCS=off", "PWD="+root,
	)
}
