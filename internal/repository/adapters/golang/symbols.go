package golang

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"golang.org/x/tools/go/packages"
)

func (state *analysisState) collectPackageFacts(fileSet *token.FileSet, pkg *packages.Package) error {
	indexed := indexedSyntaxFiles(state, pkg)
	if len(indexed) == 0 {
		return nil
	}
	state.relevantPackages[pkg.ID] = struct{}{}
	packagePath := packagePath(pkg)
	packageID, err := state.packageArtifact(packagePath, pkg.Name)
	if err != nil {
		return err
	}
	for _, item := range indexed {
		sourceID, err := state.goSourceArtifact(item.file)
		if err != nil {
			return err
		}
		state.addBuildMembership(packageID, sourceID, item.file)
		state.collectFileSymbols(fileSet, pkg, item.syntax, item.file, packagePath, packageID)
		state.collectInitializationSource(packagePath, sourceID, item.syntax)
	}
	return nil
}

type indexedSyntax struct {
	syntax *ast.File
	file   repositoryfacts.File
}

func indexedSyntaxFiles(state *analysisState, pkg *packages.Package) []indexedSyntax {
	items := make([]indexedSyntax, 0, len(pkg.Syntax))
	for index, syntax := range pkg.Syntax {
		if index >= len(pkg.CompiledGoFiles) {
			continue
		}
		file, exists := state.indexedFile(pkg.CompiledGoFiles[index])
		if !exists {
			continue
		}
		state.coveredFileIDs[file.ID] = struct{}{}
		items = append(items, indexedSyntax{syntax: syntax, file: file})
	}
	return items
}

func (state *analysisState) collectFileSymbols(
	fileSet *token.FileSet,
	pkg *packages.Package,
	fileAST *ast.File,
	file repositoryfacts.File,
	packagePath, packageID string,
) {
	for _, declaration := range fileAST.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			qualified := qualifiedFunctionName(pkg, packagePath, value)
			kind := "function"
			if value.Recv != nil {
				kind = "method"
			}
			if file.Test && goTestFunction(value.Name.Name) {
				kind = "test"
			}
			symbol := state.newSymbol(fileSet, pkg, file, kind, value.Name.Name, qualified, value, value.Name)
			state.addSymbol(symbol)
			state.addContainsEdge(packageID, symbol)
		case *ast.GenDecl:
			for _, specification := range value.Specs {
				switch spec := specification.(type) {
				case *ast.TypeSpec:
					qualified := packagePath + "." + spec.Name.Name
					symbol := state.newSymbol(fileSet, pkg, file, "type", spec.Name.Name, qualified, spec, spec.Name)
					state.addSymbol(symbol)
					state.addContainsEdge(packageID, symbol)
				case *ast.ValueSpec:
					kind := "variable"
					if value.Tok == token.CONST {
						kind = "constant"
					}
					for _, name := range spec.Names {
						qualified := packagePath + "." + name.Name
						symbol := state.newSymbol(fileSet, pkg, file, kind, name.Name, qualified, spec, name)
						state.addSymbol(symbol)
						state.addContainsEdge(packageID, symbol)
					}
				}
			}
		}
	}
}

func (state *analysisState) newSymbol(
	fileSet *token.FileSet,
	pkg *packages.Package,
	file repositoryfacts.File,
	kind, name, qualified string,
	node ast.Node,
	identifier *ast.Ident,
) repositoryfacts.Symbol {
	start, end := nodeOffsets(fileSet, node)
	signature := declarationSignature(fileSet, node)
	return repositoryfacts.NewSymbol(
		state.snapshot, file, adapterIdentity, kind, name, qualified, signature,
		start, end, repositoryfacts.OriginGoAST, 1,
	)
}

func declarationSignature(fileSet *token.FileSet, node ast.Node) string {
	if function, ok := node.(*ast.FuncDecl); ok {
		copy := *function
		copy.Doc = nil
		copy.Body = nil
		return formatNode(fileSet, &copy)
	}
	return formatNode(fileSet, node)
}

func (state *analysisState) addSymbol(symbol repositoryfacts.Symbol) {
	state.symbols[symbol.ID] = symbol
	state.symbolIDByQualified[symbol.QualifiedName] = symbol.ID
	state.symbolKindByID[symbol.ID] = symbol.Kind
}

func (state *analysisState) addContainsEdge(packageID string, symbol repositoryfacts.Symbol) {
	edge := repositoryfacts.NewEdge(
		state.snapshot, adapterIdentity, packageID, symbol.ID, "contains", symbol.FileID,
		symbol.StartByte, symbol.EndByte, repositoryfacts.OriginGoAST, 1,
	)
	state.edges[edge.ID] = edge
}

func (state *analysisState) packageArtifact(packagePath, name string) (string, error) {
	if id := state.packageArtifactIDs[packagePath]; id != "" {
		return id, nil
	}
	detail := map[string]string{}
	if strings.TrimSpace(name) != "" {
		detail["package_name"] = name
	}
	artifact, err := repositoryfacts.NewSnapshotArtifact(
		state.snapshot, "go_package", packagePath, detail,
		repositoryfacts.OriginGoPackages, adapterIdentity,
	)
	if err != nil {
		return "", err
	}
	state.artifacts[artifact.ID] = artifact
	state.packageArtifactIDs[packagePath] = artifact.ID
	return artifact.ID, nil
}

func qualifiedFunctionName(pkg *packages.Package, packagePath string, declaration *ast.FuncDecl) string {
	if pkg.TypesInfo != nil {
		if object := pkg.TypesInfo.Defs[declaration.Name]; object != nil {
			return qualifiedObjectName(object)
		}
	}
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return packagePath + "." + declaration.Name.Name
	}
	return packagePath + "." + receiverName(declaration.Recv.List[0].Type) + "." + declaration.Name.Name
}

func qualifiedObjectName(object types.Object) string {
	if object == nil {
		return ""
	}
	packagePath := "builtin"
	if object.Pkg() != nil {
		packagePath = object.Pkg().Path()
	}
	if function, ok := object.(*types.Func); ok {
		if signature, ok := function.Type().(*types.Signature); ok && signature.Recv() != nil {
			return packagePath + "." + receiverTypeName(signature.Recv().Type()) + "." + object.Name()
		}
	}
	return packagePath + "." + object.Name()
}

func receiverTypeName(value types.Type) string {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	if named, ok := value.(*types.Named); ok && named.Obj() != nil {
		return named.Obj().Name()
	}
	return types.TypeString(value, func(*types.Package) string { return "" })
}

func receiverName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverName(value.X)
	case *ast.IndexExpr:
		return receiverName(value.X)
	case *ast.IndexListExpr:
		return receiverName(value.X)
	default:
		return "receiver"
	}
}

func packagePath(pkg *packages.Package) string {
	if pkg.PkgPath != "" {
		return strings.TrimSuffix(pkg.PkgPath, ".test")
	}
	return pkg.ID
}

func packageQualifier(pkg *types.Package) string {
	if pkg == nil {
		return ""
	}
	return pkg.Path()
}

func goTestFunction(name string) bool {
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") || strings.HasPrefix(name, "Fuzz")
}

func nodeOffsets(fileSet *token.FileSet, node ast.Node) (int64, int64) {
	file := fileSet.File(node.Pos())
	if file == nil {
		return 0, 0
	}
	return int64(file.Offset(node.Pos())), int64(file.Offset(node.End()))
}

func formatNode(fileSet *token.FileSet, node ast.Node) string {
	var output bytes.Buffer
	if err := format.Node(&output, fileSet, node); err != nil {
		return ""
	}
	return output.String()
}
