package golang

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"golang.org/x/tools/go/packages"
)

func (state *analysisState) collectPackageRelations(fileSet *token.FileSet, pkg *packages.Package) error {
	indexed := indexedSyntaxFiles(state, pkg)
	if len(indexed) == 0 {
		return nil
	}
	packageID, err := state.packageArtifact(packagePath(pkg), pkg.Name)
	if err != nil {
		return err
	}
	for _, item := range indexed {
		if err := state.collectImports(fileSet, packageID, item.syntax, item.file); err != nil {
			return err
		}
		state.collectCalls(fileSet, pkg, item.syntax, item.file)
	}
	return nil
}

func (state *analysisState) collectImports(
	fileSet *token.FileSet,
	packageID string,
	fileAST *ast.File,
	file repositoryfacts.File,
) error {
	for _, specification := range fileAST.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil || strings.TrimSpace(importPath) == "" {
			continue
		}
		importID, err := state.packageArtifact(importPath, "")
		if err != nil {
			return err
		}
		start, end := nodeOffsets(fileSet, specification)
		edge := repositoryfacts.NewEdge(
			state.snapshot, adapterIdentity, packageID, importID, "imports", file.ID,
			start, end, repositoryfacts.OriginGoAST, 1,
		)
		state.edges[edge.ID] = edge
	}
	return nil
}

func (state *analysisState) collectCalls(
	fileSet *token.FileSet,
	pkg *packages.Package,
	fileAST *ast.File,
	file repositoryfacts.File,
) {
	if pkg.TypesInfo == nil {
		return
	}
	for _, declaration := range fileAST.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		callerQualified := qualifiedFunctionName(pkg, packagePath(pkg), function)
		callerID := state.symbolIDByQualified[callerQualified]
		if callerID == "" {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			called := calledObject(pkg.TypesInfo, call.Fun)
			calleeID := state.symbolIDByQualified[qualifiedObjectName(called)]
			if calleeID == "" {
				return true
			}
			start, end := nodeOffsets(fileSet, call)
			kind := "calls"
			if state.symbolKindByID[callerID] == "test" {
				kind = "tests"
			}
			edge := repositoryfacts.NewEdge(
				state.snapshot, adapterIdentity, callerID, calleeID, kind, file.ID,
				start, end, repositoryfacts.OriginGoTypes, 1,
			)
			state.edges[edge.ID] = edge
			return true
		})
	}
}

func calledObject(info *types.Info, expression ast.Expr) types.Object {
	switch value := expression.(type) {
	case *ast.Ident:
		return info.Uses[value]
	case *ast.SelectorExpr:
		if selection := info.Selections[value]; selection != nil {
			return selection.Obj()
		}
		return info.Uses[value.Sel]
	case *ast.IndexExpr:
		return calledObject(info, value.X)
	case *ast.IndexListExpr:
		return calledObject(info, value.X)
	default:
		return nil
	}
}

func (state *analysisState) collectDiagnostics(all []*packages.Package) {
	for _, pkg := range all {
		if _, relevant := state.relevantPackages[pkg.ID]; !relevant {
			continue
		}
		for _, packageError := range pkg.Errors {
			detail := strings.TrimSpace(packageError.Msg)
			if detail == "" {
				continue
			}
			diagnostic := repositoryfacts.AnalysisDiagnostic{
				Severity: "error", Subject: packagePath(pkg), Detail: detail,
			}
			key := diagnostic.Subject + "\x00" + diagnostic.Detail
			state.diagnostics[key] = diagnostic
		}
	}
}
