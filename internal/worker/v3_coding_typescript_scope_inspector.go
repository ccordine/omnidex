package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/modelcontext"
)

const (
	directCodingTypeScriptScopeInspectorFile      = ".omnidex-typescript-scope.mjs"
	directCodingTypeScriptScopeInspectorSchema    = "omnidex.typescript-lexical-scope.v4"
	maxDirectCodingTypeScriptDeterministicRepairs = 8
)

type directCodingTypeScriptDeterministicRepairMechanism string

const (
	directCodingTypeScriptPrimitiveNullishNarrowing   directCodingTypeScriptDeterministicRepairMechanism = "deterministic_primitive_nullish_narrowing"
	directCodingTypeScriptPrimitiveReferenceNarrowing directCodingTypeScriptDeterministicRepairMechanism = "deterministic_primitive_reference_narrowing"
)

type directCodingTypeScriptScopeReceipt struct {
	Schema               *string                                            `json:"schema"`
	Bindings             *[]assemblyline.TypeScriptRepairBinding            `json:"bindings"`
	UnavailableBindings  *[]assemblyline.TypeScriptRepairBinding            `json:"unavailable_bindings"`
	ExpressionEvidence   *[]assemblyline.TypeScriptRepairExpressionEvidence `json:"expression_evidence"`
	DeterministicRepairs *[]directCodingTypeScriptDeterministicRepair       `json:"deterministic_repairs"`
}

type directCodingTypeScriptDeterministicRepair struct {
	Mechanism              directCodingTypeScriptDeterministicRepairMechanism `json:"mechanism"`
	EvidenceIndex          int                                                `json:"evidence_index"`
	Source                 string                                             `json:"source"`
	Replacement            string                                             `json:"replacement"`
	StartByte              int                                                `json:"start_byte"`
	EndByte                int                                                `json:"end_byte"`
	NormalizationStartByte *int                                               `json:"normalization_start_byte"`
}

type directCodingTypeScriptScope struct {
	Bindings             []assemblyline.TypeScriptRepairBinding
	UnavailableBindings  []assemblyline.TypeScriptRepairBinding
	ExpressionEvidence   []assemblyline.TypeScriptRepairExpressionEvidence
	DeterministicRepairs []directCodingTypeScriptDeterministicRepair
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
	if err := validateDirectCodingTypeScriptScopeInspector(root); err != nil {
		return directCodingTypeScriptScope{}, err
	}
	args := typeScriptScopeInspectorNodeArgs(
		filepath.ToSlash(path), diagnostic.DocumentLine, diagnostic.DocumentColumn,
		diagnostic.DocumentBlockStartLine, diagnostic.DocumentBlockEndLine,
	)
	output, err := runDirectCodingStageCommand(
		ctx, root, directCodingStageTimeout, "node", args...,
	)
	if err != nil {
		return directCodingTypeScriptScope{}, fmt.Errorf("inspect TypeScript compiler scope: %w\n%s", err, trimForBudget(output, 12_000))
	}
	scope, err := decodeDirectCodingTypeScriptScopeReceipt([]byte(output))
	if err != nil {
		return directCodingTypeScriptScope{}, err
	}
	projected, err := projectDirectCodingTypeScriptCompilerScopeForModel(scope)
	if err != nil {
		return directCodingTypeScriptScope{}, err
	}
	return projected, nil
}

func validateDirectCodingTypeScriptScopeInspector(root string) error {
	path := filepath.Join(root, directCodingTypeScriptScopeInspectorFile)
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("validate TypeScript scope inspector: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("TypeScript scope inspector is not the exact code-owned regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read TypeScript scope inspector: %w", err)
	}
	if !bytes.Equal(raw, []byte(directCodingTypeScriptScopeInspectorSource)) {
		return fmt.Errorf("TypeScript scope inspector differs from its code-owned source")
	}
	return nil
}

// projectDirectCodingTypeScriptCompilerScope reduces a compiler-proven union
// mismatch to the first enclosing expression that the checker proved is not
// assignable to its contextual type. The inspector orders expressions by the
// exact diagnostic coordinate. Code also retains only the local bindings that
// the checker resolved inside that expression. Other compiler failure classes
// keep their complete scope because no equivalent deterministic projection has
// been established for them.
func projectDirectCodingTypeScriptCompilerScope(
	scope directCodingTypeScriptScope,
) (directCodingTypeScriptScope, error) {
	selectedIndex := -1
	for index, evidence := range scope.ExpressionEvidence {
		if evidence.ContextualType != "" && len(evidence.IncompatibleTypes) > 0 {
			selectedIndex = index
			break
		}
	}
	if selectedIndex < 0 || len(scope.ExpressionEvidence[selectedIndex].ReferencedBindings) == 0 {
		return scope, nil
	}
	selected := scope.ExpressionEvidence[selectedIndex]
	selectedRepairs := make([]directCodingTypeScriptDeterministicRepair, 0, 1)
	for _, repair := range scope.DeterministicRepairs {
		if repair.EvidenceIndex == selectedIndex {
			selectedRepairs = append(selectedRepairs, repair)
		}
	}
	if len(selectedRepairs) > 1 {
		return directCodingTypeScriptScope{}, fmt.Errorf(
			"TypeScript compiler projection found %d deterministic repairs for one expression",
			len(selectedRepairs),
		)
	}
	wanted := make(map[string]struct{}, len(selected.ReferencedBindings))
	for _, name := range selected.ReferencedBindings {
		wanted[name] = struct{}{}
	}
	bindings := make([]assemblyline.TypeScriptRepairBinding, 0, len(wanted))
	for _, binding := range scope.Bindings {
		if _, required := wanted[binding.Name]; required {
			bindings = append(bindings, binding)
		}
	}
	if len(bindings) != len(wanted) {
		return directCodingTypeScriptScope{}, fmt.Errorf(
			"TypeScript compiler expression references %d bindings but exact scope resolved %d",
			len(wanted), len(bindings),
		)
	}
	return directCodingTypeScriptScope{
		Bindings:             bindings,
		ExpressionEvidence:   []assemblyline.TypeScriptRepairExpressionEvidence{selected},
		DeterministicRepairs: selectedRepairs,
	}, nil
}

func projectDirectCodingTypeScriptCompilerScopeForModel(
	scope directCodingTypeScriptScope,
) (directCodingTypeScriptScope, error) {
	projected, err := projectDirectCodingTypeScriptCompilerScope(scope)
	if err != nil {
		return directCodingTypeScriptScope{}, err
	}
	if err := validateDirectCodingTypeScriptCompilerScopeModelProjection(projected); err != nil {
		return directCodingTypeScriptScope{}, err
	}
	return projected, nil
}

func applyDirectCodingTypeScriptDeterministicRepair(
	current string,
	scope directCodingTypeScriptScope,
) (string, bool, error) {
	if len(scope.DeterministicRepairs) == 0 {
		return current, false, nil
	}
	if len(scope.DeterministicRepairs) != 1 {
		return "", false, fmt.Errorf(
			"TypeScript compiler scope contains %d deterministic repairs; expected at most one",
			len(scope.DeterministicRepairs),
		)
	}
	repair := scope.DeterministicRepairs[0]
	if !validDirectCodingTypeScriptDeterministicRepairMechanism(repair.Mechanism) {
		return "", false, fmt.Errorf("TypeScript deterministic repair mechanism %q is invalid", repair.Mechanism)
	}
	if repair.StartByte < 0 || repair.EndByte <= repair.StartByte || repair.EndByte > len(current) {
		return "", false, fmt.Errorf("TypeScript deterministic repair byte range is outside the current declaration")
	}
	if current[repair.StartByte:repair.EndByte] != repair.Source {
		return "", false, fmt.Errorf("TypeScript deterministic repair source no longer matches the current declaration")
	}
	if repair.NormalizationStartByte == nil {
		return "", false, fmt.Errorf("TypeScript deterministic repair lacks exact normalization occurrence authority")
	}
	normalizationStart := *repair.NormalizationStartByte
	if repair.Mechanism == directCodingTypeScriptPrimitiveNullishNarrowing {
		if normalizationStart != repair.StartByte {
			return "", false, fmt.Errorf("TypeScript nullish repair normalization occurrence does not match its target")
		}
	} else {
		if normalizationStart < 0 || len(repair.Replacement) > repair.StartByte ||
			normalizationStart > repair.StartByte-len(repair.Replacement) {
			return "", false, fmt.Errorf("TypeScript reference repair normalization occurrence is outside its prior authority")
		}
		normalizationEnd := normalizationStart + len(repair.Replacement)
		if current[normalizationStart:normalizationEnd] != repair.Replacement {
			return "", false, fmt.Errorf("TypeScript reference repair normalization occurrence no longer matches the current declaration")
		}
	}
	candidate := current[:repair.StartByte] + repair.Replacement + current[repair.EndByte:]
	if candidate == current {
		return "", false, fmt.Errorf("TypeScript deterministic repair produced no source transition")
	}
	return candidate, true, nil
}

func directCodingTypeScriptRepairRegionHasExactIncompatibility(
	region *assemblyline.TypeScriptFragmentRepairRegion,
) bool {
	return region != nil &&
		region.Kind == assemblyline.TypeScriptRepairRegionCompilerOwner &&
		len(region.ExpressionEvidence) == 1 &&
		region.ExpressionEvidence[0].ContextualType != "" &&
		len(region.ExpressionEvidence[0].IncompatibleTypes) > 0
}

// validateDirectCodingTypeScriptCompilerScopeModelProjection verifies the
// compiler-owned additions to an already-authorized mutable declaration. An
// expression Source is an exact substring of that declaration, which the
// repair region already carries as source authority. It is code, not a file
// identity: it can legitimately contain regular-expression escapes or URL
// literals. Compiler-rendered type information and binding metadata, however,
// are new model-visible projections and must remain path-free. Binding names
// retain the strict prose boundary; type, signature, and member fields use
// source grammar so ordinary parameter labels are not mistaken for drives.
func validateDirectCodingTypeScriptCompilerScopeModelProjection(
	scope directCodingTypeScriptScope,
) error {
	allBindings := append(
		append([]assemblyline.TypeScriptRepairBinding(nil), scope.Bindings...),
		scope.UnavailableBindings...,
	)
	for _, binding := range allBindings {
		if directCodingTypeScriptCompilerContainsPathIdentity(binding.Name) {
			return fmt.Errorf("TypeScript compiler scope binding %s contains path identity", binding.Name)
		}
		values := append([]string{binding.Type}, binding.CallableSignatures...)
		values = append(values, binding.Members...)
		for _, value := range values {
			if directCodingTypeScriptCompilerSyntaxContainsPathIdentity(value) {
				return fmt.Errorf("TypeScript compiler scope binding %s contains path identity", binding.Name)
			}
		}
	}
	for index, item := range scope.ExpressionEvidence {
		values := append([]string{item.InferredType, item.ContextualType}, item.IncompatibleTypes...)
		for _, value := range values {
			if value != "" && directCodingTypeScriptCompilerSyntaxContainsPathIdentity(value) {
				return fmt.Errorf(
					"TypeScript compiler expression evidence %d contains path identity", index+1,
				)
			}
		}
	}
	return nil
}

func directCodingTypeScriptCompilerSyntaxContainsPathIdentity(value string) bool {
	return len(modelcontext.SourcePathIdentities(
		value, modelcontext.ArtifactIdentityProvenance{},
	)) > 0
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
		receipt.ExpressionEvidence == nil || receipt.DeterministicRepairs == nil {
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
	for evidenceIndex, evidence := range expressionEvidence {
		for _, name := range evidence.ReferencedBindings {
			if _, exists := availableNames[name]; !exists {
				return directCodingTypeScriptScope{}, fmt.Errorf(
					"TypeScript compiler expression evidence %d references unknown local binding %q",
					evidenceIndex+1, name,
				)
			}
		}
	}
	deterministicRepairs := append(
		[]directCodingTypeScriptDeterministicRepair(nil), (*receipt.DeterministicRepairs)...,
	)
	if len(deterministicRepairs) > maxDirectCodingTypeScriptDeterministicRepairs {
		return directCodingTypeScriptScope{}, fmt.Errorf(
			"TypeScript compiler scope receipt contains %d deterministic repairs; maximum is %d",
			len(deterministicRepairs), maxDirectCodingTypeScriptDeterministicRepairs,
		)
	}
	seenRepairEvidence := make(map[int]struct{}, len(deterministicRepairs))
	for index, repair := range deterministicRepairs {
		if !validDirectCodingTypeScriptDeterministicRepairMechanism(repair.Mechanism) ||
			repair.NormalizationStartByte == nil ||
			repair.Source == "" || repair.Replacement == "" ||
			!utf8.ValidString(repair.Source) || !utf8.ValidString(repair.Replacement) ||
			strings.ContainsAny(repair.Source, "\r\n") || strings.ContainsAny(repair.Replacement, "\r\n") ||
			repair.EvidenceIndex < 0 || repair.EvidenceIndex >= len(expressionEvidence) ||
			repair.StartByte < 0 || repair.EndByte-repair.StartByte != len(repair.Source) ||
			expressionEvidence[repair.EvidenceIndex].Source != repair.Source {
			return directCodingTypeScriptScope{}, fmt.Errorf(
				"TypeScript compiler deterministic repair %d is invalid", index+1,
			)
		}
		normalizationStart := *repair.NormalizationStartByte
		if normalizationStart < 0 ||
			(repair.Mechanism == directCodingTypeScriptPrimitiveNullishNarrowing && normalizationStart != repair.StartByte) ||
			(repair.Mechanism == directCodingTypeScriptPrimitiveReferenceNarrowing && normalizationStart >= repair.StartByte) {
			return directCodingTypeScriptScope{}, fmt.Errorf(
				"TypeScript compiler deterministic repair %d has invalid normalization occurrence authority", index+1,
			)
		}
		if _, duplicate := seenRepairEvidence[repair.EvidenceIndex]; duplicate {
			return directCodingTypeScriptScope{}, fmt.Errorf(
				"TypeScript compiler deterministic repair %d repeats evidence index %d",
				index+1, repair.EvidenceIndex,
			)
		}
		seenRepairEvidence[repair.EvidenceIndex] = struct{}{}
	}
	return directCodingTypeScriptScope{
		Bindings: bindings, UnavailableBindings: unavailable,
		ExpressionEvidence: expressionEvidence, DeterministicRepairs: deterministicRepairs,
	}, nil
}

func validDirectCodingTypeScriptDeterministicRepairMechanism(
	mechanism directCodingTypeScriptDeterministicRepairMechanism,
) bool {
	switch mechanism {
	case directCodingTypeScriptPrimitiveNullishNarrowing,
		directCodingTypeScriptPrimitiveReferenceNarrowing:
		return true
	default:
		return false
	}
}
