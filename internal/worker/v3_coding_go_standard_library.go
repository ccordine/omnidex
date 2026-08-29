package worker

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/gofragment"
)

type directCodingGoStandardLibraryCapability struct {
	ID          string
	Purpose     string
	ImportPath  string
	PackageName string
	SymbolName  string
	API         string
	Source      string
}

var directCodingGoStandardLibraryRegistry = []directCodingGoStandardLibraryCapability{
	{
		ID: "runtime.stdlib.environment_value", ImportPath: "os",
		Purpose:     "Read one process environment value and report whether it is defined.",
		PackageName: "os", SymbolName: "LookupEnv",
		API:    "func RuntimeEnvironmentValue(string) (string, bool)",
		Source: "func RuntimeEnvironmentValue(key string) (string, bool) { return os.LookupEnv(key) }",
	},
	{
		ID: "runtime.stdlib.read_file", ImportPath: "os",
		Purpose:     "Read all bytes from one user-supplied local file name.",
		PackageName: "os", SymbolName: "ReadFile",
		API:    "func RuntimeReadFile(string) ([]byte, error)",
		Source: "func RuntimeReadFile(name string) ([]byte, error) { return os.ReadFile(name) }",
	},
}

func registeredDirectCodingGoStandardLibraryCapabilities() (
	[]directCodingGoStandardLibraryCapability,
	error,
) {
	capabilities := append(
		[]directCodingGoStandardLibraryCapability(nil),
		directCodingGoStandardLibraryRegistry...,
	)
	sort.Slice(capabilities, func(left, right int) bool {
		return capabilities[left].ID < capabilities[right].ID
	})
	seenIDs := make(map[string]struct{}, len(capabilities))
	standardLibraryImporter := importer.Default()
	for index, capability := range capabilities {
		if capability.ID == "" || capability.ID != strings.TrimSpace(capability.ID) {
			return nil, fmt.Errorf("Go standard-library capability %d has an invalid ID", index)
		}
		if _, duplicate := seenIDs[capability.ID]; duplicate {
			return nil, fmt.Errorf("Go standard-library capability ID %q is registered more than once", capability.ID)
		}
		seenIDs[capability.ID] = struct{}{}
		if capability.Purpose == "" || capability.Purpose != strings.TrimSpace(capability.Purpose) ||
			strings.ContainsAny(capability.Purpose, "\x00\r\n") {
			return nil, fmt.Errorf("Go standard-library capability %s has invalid semantic purpose", capability.ID)
		}
		if capability.ImportPath == "" || capability.ImportPath != path.Clean(capability.ImportPath) ||
			capability.PackageName == "" || capability.PackageName != strings.TrimSpace(capability.PackageName) ||
			capability.SymbolName == "" || capability.SymbolName != strings.TrimSpace(capability.SymbolName) {
			return nil, fmt.Errorf("Go standard-library capability %s has invalid package authority", capability.ID)
		}
		if err := validateDirectCodingGoStandardLibraryCapability(
			standardLibraryImporter, capability,
		); err != nil {
			return nil, fmt.Errorf("validate Go standard-library capability %s: %w", capability.ID, err)
		}
	}
	return capabilities, nil
}

func directCodingGoRuntimeCapabilities() ([]directCodingRuntimeCapability, error) {
	registered, err := registeredDirectCodingGoStandardLibraryCapabilities()
	if err != nil {
		return nil, err
	}
	capabilities := make([]directCodingRuntimeCapability, len(registered))
	for index, capability := range registered {
		capabilities[index] = directCodingRuntimeCapability{
			ID: capability.ID, Purpose: capability.Purpose,
		}
	}
	if err := validateDirectCodingRuntimeCapabilityRegistry(capabilities); err != nil {
		return nil, err
	}
	return capabilities, nil
}

func validateDirectCodingGoStandardLibraryCapability(
	standardLibraryImporter types.Importer,
	capability directCodingGoStandardLibraryCapability,
) error {
	if standardLibraryImporter == nil {
		return fmt.Errorf("standard-library importer is unavailable")
	}
	apiSignature, err := gofragment.CompileNewFunctionSignature(capability.API)
	if err != nil {
		return fmt.Errorf("compile wrapper API signature: %w", err)
	}
	sourceSignature, err := gofragment.ExtractUniqueNewFunctionSignature(capability.Source)
	if err != nil {
		return fmt.Errorf("extract wrapper source signature: %w", err)
	}
	if apiSignature.Name != sourceSignature.Name {
		return fmt.Errorf(
			"wrapper API name %s differs from source name %s",
			apiSignature.Name, sourceSignature.Name,
		)
	}
	if _, err := gofragment.ParseNewFunction(
		sourceSignature.Canonical,
		[]string{capability.PackageName + "." + capability.SymbolName},
		capability.Source,
	); err != nil {
		return fmt.Errorf("validate wrapper source authority: %w", err)
	}
	sourceText := "package main\n\nimport " + strconv.Quote(capability.ImportPath) + "\n\n" + capability.Source
	sourceType, parsed, err := directCodingGoCheckedFunctionSignature(
		types.Config{Importer: standardLibraryImporter}, sourceText, apiSignature.Name,
	)
	if err != nil {
		return fmt.Errorf("type-check wrapper source: %w", err)
	}
	apiText := "package main\n\n" + apiSignature.Canonical + ` { panic("code-owned type-shape probe") }`
	apiType, apiParsed, err := directCodingGoCheckedFunctionSignature(
		types.Config{}, apiText, apiSignature.Name,
	)
	if err != nil {
		return fmt.Errorf("type-check wrapper API: %w", err)
	}
	if err := validateDirectCodingGoModelVisibleFunctionAPI(
		apiParsed, apiSignature.Name,
	); err != nil {
		return err
	}
	if !types.Identical(apiType, sourceType) {
		return fmt.Errorf(
			"wrapper API type %s differs from source type %s", apiType, sourceType,
		)
	}
	selected := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil || selector.Sel.Name != capability.SymbolName {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && identifier.Name == capability.PackageName {
			selected++
		}
		return true
	})
	if selected != 1 {
		return fmt.Errorf(
			"wrapper must reference %s.%s exactly once, found %d",
			capability.PackageName, capability.SymbolName, selected,
		)
	}
	return nil
}

func validateDirectCodingGoModelVisibleFunctionAPI(
	parsed *ast.File,
	functionName string,
) error {
	if parsed == nil {
		return fmt.Errorf("wrapper API syntax tree is unavailable")
	}
	for _, candidate := range parsed.Decls {
		function, ok := candidate.(*ast.FuncDecl)
		if !ok || function.Name == nil || function.Name.Name != functionName {
			continue
		}
		for _, fields := range []*ast.FieldList{function.Type.Params, function.Type.Results} {
			if fields == nil {
				continue
			}
			for _, field := range fields.List {
				if len(field.Names) != 0 {
					return fmt.Errorf(
						"wrapper API %s must omit parameter and result names from model-visible authority",
						functionName,
					)
				}
			}
		}
		return nil
	}
	return fmt.Errorf("wrapper API function %s is unavailable", functionName)
}

func directCodingGoCheckedFunctionSignature(
	typeConfig types.Config,
	source string,
	functionName string,
) (*types.Signature, *ast.File, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "", source, parser.AllErrors)
	if err != nil {
		return nil, nil, err
	}
	definitions := make(map[*ast.Ident]types.Object)
	if _, err := typeConfig.Check(
		"example.invalid/omnidex/runtime-capability", fileSet, []*ast.File{parsed},
		&types.Info{Defs: definitions},
	); err != nil {
		return nil, nil, err
	}
	var declaration *ast.FuncDecl
	for _, candidate := range parsed.Decls {
		function, ok := candidate.(*ast.FuncDecl)
		if !ok || function.Name == nil || function.Name.Name != functionName {
			continue
		}
		if declaration != nil {
			return nil, nil, fmt.Errorf("function %s is declared more than once", functionName)
		}
		declaration = function
	}
	if declaration == nil {
		return nil, nil, fmt.Errorf("function %s is not declared", functionName)
	}
	object, ok := definitions[declaration.Name].(*types.Func)
	if !ok {
		return nil, nil, fmt.Errorf("function %s has no typed declaration", functionName)
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok {
		return nil, nil, fmt.Errorf("function %s has no typed signature", functionName)
	}
	return signature, parsed, nil
}
