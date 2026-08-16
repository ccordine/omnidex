package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	directCodingTypeScriptScopeInspectorFile   = ".omnidex-typescript-scope.mjs"
	directCodingTypeScriptScopeInspectorSchema = "omnidex.typescript-lexical-scope.v2"
)

type directCodingTypeScriptScopeReceipt struct {
	Schema              *string                                            `json:"schema"`
	Bindings            *[]assemblyline.TypeScriptRepairBinding            `json:"bindings"`
	UnavailableBindings *[]assemblyline.TypeScriptRepairBinding            `json:"unavailable_bindings"`
	ExpressionEvidence  *[]assemblyline.TypeScriptRepairExpressionEvidence `json:"expression_evidence"`
}

type directCodingTypeScriptScope struct {
	Bindings            []assemblyline.TypeScriptRepairBinding
	UnavailableBindings []assemblyline.TypeScriptRepairBinding
	ExpressionEvidence  []assemblyline.TypeScriptRepairExpressionEvidence
}

func writeDirectCodingTypeScriptScopeInspector(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("write TypeScript scope inspector requires one absolute stage root")
	}
	if err := os.WriteFile(
		filepath.Join(root, directCodingTypeScriptScopeInspectorFile),
		[]byte(directCodingTypeScriptScopeInspectorSource), 0o600,
	); err != nil {
		return fmt.Errorf("write TypeScript scope inspector: %w", err)
	}
	return nil
}

func inspectDirectCodingTypeScriptScope(
	ctx context.Context,
	root string,
	diagnostic directCodingStageDiagnostic,
) (directCodingTypeScriptScope, error) {
	if ctx == nil || root == "" || !filepath.IsAbs(root) || !diagnostic.CompilerIssue {
		return directCodingTypeScriptScope{}, fmt.Errorf("inspect TypeScript scope requires compiler-owned stage authority")
	}
	path := filepath.Clean(filepath.FromSlash(diagnostic.DocumentPath))
	if path == "." || filepath.IsAbs(path) || path == ".." ||
		strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return directCodingTypeScriptScope{}, fmt.Errorf("inspect TypeScript scope requires one stage-relative document path")
	}
	if diagnostic.DocumentLine < 1 || diagnostic.DocumentColumn < 1 ||
		diagnostic.DocumentBlockStartLine < 1 ||
		diagnostic.DocumentBlockEndLine < diagnostic.DocumentBlockStartLine ||
		diagnostic.DocumentLine < diagnostic.DocumentBlockStartLine ||
		diagnostic.DocumentLine > diagnostic.DocumentBlockEndLine {
		return directCodingTypeScriptScope{}, fmt.Errorf("inspect TypeScript scope received invalid document coordinates")
	}
	output, err := runDirectCodingStageCommand(
		ctx, root, directCodingStageTimeout, "node", directCodingTypeScriptScopeInspectorFile,
		filepath.ToSlash(path), strconv.Itoa(diagnostic.DocumentLine),
		strconv.Itoa(diagnostic.DocumentColumn), strconv.Itoa(diagnostic.DocumentBlockStartLine),
		strconv.Itoa(diagnostic.DocumentBlockEndLine),
	)
	if err != nil {
		return directCodingTypeScriptScope{}, fmt.Errorf("inspect TypeScript compiler scope: %w\n%s", err, trimForBudget(output, 12_000))
	}
	scope, err := decodeDirectCodingTypeScriptScopeReceipt([]byte(output))
	if err != nil {
		return directCodingTypeScriptScope{}, err
	}
	allBindings := append(
		append([]assemblyline.TypeScriptRepairBinding(nil), scope.Bindings...),
		scope.UnavailableBindings...,
	)
	for _, binding := range allBindings {
		values := append([]string{binding.Name, binding.Type}, binding.CallableSignatures...)
		values = append(values, binding.Members...)
		for _, value := range values {
			if directCodingTypeScriptCompilerContainsPathIdentity(value) {
				return directCodingTypeScriptScope{}, fmt.Errorf("TypeScript compiler scope binding %s contains path identity", binding.Name)
			}
		}
	}
	for index, item := range scope.ExpressionEvidence {
		values := append(
			[]string{item.Source, item.InferredType, item.ContextualType},
			item.IncompatibleTypes...,
		)
		for _, value := range values {
			if value != "" && directCodingTypeScriptCompilerContainsPathIdentity(value) {
				return directCodingTypeScriptScope{}, fmt.Errorf(
					"TypeScript compiler expression evidence %d contains path identity", index+1,
				)
			}
		}
	}
	return scope, nil
}

func decodeDirectCodingTypeScriptScopeReceipt(raw []byte) (directCodingTypeScriptScope, error) {
	var receipt directCodingTypeScriptScopeReceipt
	if err := exactjson.ValidateObject(raw, receipt, "TypeScript compiler scope receipt"); err != nil {
		return directCodingTypeScriptScope{}, fmt.Errorf("decode TypeScript compiler scope receipt: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return directCodingTypeScriptScope{}, fmt.Errorf("decode TypeScript compiler scope receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return directCodingTypeScriptScope{}, fmt.Errorf("decode TypeScript compiler scope receipt trailing data")
	}
	if receipt.Schema == nil || *receipt.Schema != directCodingTypeScriptScopeInspectorSchema ||
		receipt.Bindings == nil || receipt.UnavailableBindings == nil ||
		receipt.ExpressionEvidence == nil {
		return directCodingTypeScriptScope{}, fmt.Errorf(
			"TypeScript compiler scope receipt lacks exact schema and binding inventories",
		)
	}
	bindings := append([]assemblyline.TypeScriptRepairBinding(nil), (*receipt.Bindings)...)
	if len(bindings) == 0 {
		return directCodingTypeScriptScope{}, fmt.Errorf("TypeScript compiler scope receipt contains no local bindings")
	}
	if err := assemblyline.ValidateExactTypeScriptRepairBindings(bindings); err != nil {
		return directCodingTypeScriptScope{}, fmt.Errorf("validate TypeScript compiler scope receipt: %w", err)
	}
	unavailable := append(
		[]assemblyline.TypeScriptRepairBinding(nil), (*receipt.UnavailableBindings)...,
	)
	if err := assemblyline.ValidateExactTypeScriptRepairBindings(unavailable); err != nil {
		return directCodingTypeScriptScope{}, fmt.Errorf(
			"validate TypeScript compiler unavailable scope receipt: %w", err,
		)
	}
	expressionEvidence := append(
		[]assemblyline.TypeScriptRepairExpressionEvidence(nil),
		(*receipt.ExpressionEvidence)...,
	)
	if len(expressionEvidence) == 0 {
		return directCodingTypeScriptScope{}, fmt.Errorf(
			"TypeScript compiler scope receipt contains no expression evidence",
		)
	}
	if err := assemblyline.ValidateTypeScriptRepairExpressionEvidence(expressionEvidence); err != nil {
		return directCodingTypeScriptScope{}, fmt.Errorf(
			"validate TypeScript compiler expression evidence: %w", err,
		)
	}
	availableNames := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		availableNames[binding.Name] = struct{}{}
	}
	for _, binding := range unavailable {
		if _, duplicate := availableNames[binding.Name]; duplicate {
			return directCodingTypeScriptScope{}, fmt.Errorf(
				"TypeScript compiler scope binding %q is both available and unavailable",
				binding.Name,
			)
		}
	}
	return directCodingTypeScriptScope{
		Bindings: bindings, UnavailableBindings: unavailable,
		ExpressionEvidence: expressionEvidence,
	}, nil
}
