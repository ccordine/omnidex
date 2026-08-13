package golang

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"golang.org/x/tools/go/packages"
)

func (state *analysisState) goSourceArtifact(file repositoryfacts.File) (string, error) {
	if file.Language != "go" {
		return "", fmt.Errorf("Go source artifact requires an exact indexed Go file")
	}
	return state.fileArtifact(file, "go_source")
}

func (state *analysisState) fileArtifact(file repositoryfacts.File, kind string) (string, error) {
	if id := state.fileArtifactIDs[file.ID]; id != "" {
		return id, nil
	}
	detail := map[string]string{}
	if kind == "go_source" {
		detail["build_class"] = "production"
		if strings.HasSuffix(file.Path, "_test.go") {
			detail["build_class"] = "test"
		}
	}
	artifact, err := repositoryfacts.NewFileArtifact(
		state.snapshot, file, kind, file.ID, detail,
		repositoryfacts.OriginGoPackages, adapterIdentity,
	)
	if err != nil {
		return "", err
	}
	state.artifacts[artifact.ID] = artifact
	state.fileArtifactIDs[file.ID] = artifact.ID
	return artifact.ID, nil
}

func (state *analysisState) addBuildMembership(
	packageID, sourceID string,
	file repositoryfacts.File,
) {
	edge := repositoryfacts.NewEdge(
		state.snapshot, adapterIdentity, packageID, sourceID, "builds_from", file.ID,
		0, file.Size, repositoryfacts.OriginGoPackages, 1,
	)
	state.edges[edge.ID] = edge
}

func (state *analysisState) collectInitializationSource(
	packagePath, sourceID string,
	fileAST *ast.File,
) {
	for _, specification := range fileAST.Imports {
		if specification.Name != nil && specification.Name.Name == "_" {
			state.addInitializationSource(packagePath, sourceID)
			return
		}
	}
	for _, declaration := range fileAST.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			if value.Recv == nil && value.Name.Name == "init" {
				state.addInitializationSource(packagePath, sourceID)
				return
			}
		case *ast.GenDecl:
			if value.Tok != token.VAR {
				continue
			}
			for _, specification := range value.Specs {
				if variable, ok := specification.(*ast.ValueSpec); ok && len(variable.Values) > 0 {
					state.addInitializationSource(packagePath, sourceID)
					return
				}
			}
		}
	}
}

func (state *analysisState) addInitializationSource(packagePath, sourceID string) {
	if state.initializationSourcesByPackage[packagePath] == nil {
		state.initializationSourcesByPackage[packagePath] = make(map[string]struct{})
	}
	state.initializationSourcesByPackage[packagePath][sourceID] = struct{}{}
}

func (state *analysisState) addInitializationEdges(
	importPath, importerID, evidenceFileID string,
	start, end int64,
	kind string,
) {
	sources := state.initializationSourcesByPackage[importPath]
	ids := make([]string, 0, len(sources))
	for id := range sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, sourceID := range ids {
		edge := repositoryfacts.NewEdge(
			state.snapshot, adapterIdentity, importerID, sourceID, kind,
			evidenceFileID, start, end, repositoryfacts.OriginGoTypes, 1,
		)
		state.edges[edge.ID] = edge
	}
}

func (state *analysisState) collectOpaqueBuildInputs(packageID string, pkg *packages.Package) error {
	for _, absolute := range pkg.OtherFiles {
		file, exists := state.allFilesByAbsolute[filepath.Clean(absolute)]
		if !exists {
			return fmt.Errorf("Go package %q has an opaque build input outside the exact repository snapshot", packagePath(pkg))
		}
		inputID, err := state.fileArtifact(file, "go_opaque_build_input")
		if err != nil {
			return err
		}
		edge := repositoryfacts.NewEdge(
			state.snapshot, adapterIdentity, packageID, inputID, "builds_from_opaque", "",
			0, 0, repositoryfacts.OriginGoPackages, 1,
		)
		state.edges[edge.ID] = edge
	}
	return nil
}

func (state *analysisState) collectEmbedInputs(packageID string, pkg *packages.Package) error {
	for _, absolute := range pkg.EmbedFiles {
		file, exists := state.allFilesByAbsolute[filepath.Clean(absolute)]
		if !exists {
			return fmt.Errorf("Go package %q has an embedded build input outside the exact repository snapshot", packagePath(pkg))
		}
		kind := "go_build_input"
		if file.Language == "go" {
			kind = "go_source"
		}
		inputID, err := state.fileArtifact(file, kind)
		if err != nil {
			return err
		}
		edge := repositoryfacts.NewEdge(
			state.snapshot, adapterIdentity, packageID, inputID, "embeds", "",
			0, 0, repositoryfacts.OriginGoPackages, 1,
		)
		state.edges[edge.ID] = edge
	}
	return nil
}
