package golang

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"golang.org/x/tools/go/packages"
)

func (state *analysisState) collectTypedUses(
	fileSet *token.FileSet,
	pkg *packages.Package,
	fileAST *ast.File,
	file repositoryfacts.File,
) error {
	if pkg.TypesInfo == nil {
		return fmt.Errorf("Go package %q has no typed-use authority", packagePath(pkg))
	}
	sourceID := state.fileArtifactIDs[file.ID]
	if sourceID == "" {
		return fmt.Errorf("Go source %q has no exact source artifact authority", file.ID)
	}
	ast.Inspect(fileAST, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := pkg.TypesInfo.Uses[identifier]
		targetID := state.symbolIDByQualified[qualifiedObjectName(object)]
		if targetID == "" {
			return true
		}
		kind := typedUseKind(object)
		if kind == "" {
			return true
		}
		start, end := nodeOffsets(fileSet, identifier)
		edge := repositoryfacts.NewEdge(
			state.snapshot, adapterIdentity, sourceID, targetID, kind, file.ID,
			start, end, repositoryfacts.OriginGoTypes, 1,
		)
		state.edges[edge.ID] = edge
		return true
	})
	return nil
}

func typedUseKind(object types.Object) string {
	switch object.(type) {
	case *types.TypeName:
		if !packageScopeObject(object) {
			return ""
		}
		return "uses_type"
	case *types.Const, *types.Var:
		if !packageScopeObject(object) {
			return ""
		}
		return "uses_value"
	case *types.Func:
		return "uses_value"
	default:
		return ""
	}
}

func packageScopeObject(object types.Object) bool {
	return object != nil && object.Pkg() != nil && object.Parent() == object.Pkg().Scope()
}
