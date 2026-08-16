package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	ChangeContractSchemaV2    = "omnidex.repository-change-contract.v2"
	maxChangeTargets          = 8
	maxChangeSymbolBytes      = 64 * 1024
	maxChangeRequirementBytes = 512
	maxDirectCapabilities     = 32
	maxDirectCapabilityBytes  = 2048
	maxPermittedSymbolBytes   = 1024
	maxVerificationSymbols    = 64
)

type ChangeRequest struct {
	SymbolID         string `json:"symbol_id"`
	RequirementQuote string `json:"requirement_quote"`
}

type DirectCapability struct {
	SymbolID         string   `json:"symbol_id"`
	Name             string   `json:"name"`
	Signature        string   `json:"signature"`
	SourceSHA256     string   `json:"source_sha256"`
	PermittedSymbols []string `json:"permitted_symbols"`
}

type ChangeTarget struct {
	SymbolID                  string             `json:"symbol_id"`
	FileID                    string             `json:"file_id"`
	Kind                      string             `json:"kind"`
	Signature                 string             `json:"signature"`
	StartByte                 int64              `json:"start_byte"`
	EndByte                   int64              `json:"end_byte"`
	ExpectedFileSHA256        string             `json:"expected_file_sha256"`
	ExpectedDeclarationSHA256 string             `json:"expected_declaration_sha256"`
	RequirementQuote          string             `json:"requirement_quote"`
	DirectCapabilities        []DirectCapability `json:"direct_capabilities"`
	VerificationSymbolIDs     []string           `json:"verification_symbol_ids"`
}

type ChangeContract struct {
	Schema     string         `json:"schema"`
	ID         string         `json:"id"`
	SnapshotID string         `json:"snapshot_id"`
	AnalysisID string         `json:"analysis_id"`
	Targets    []ChangeTarget `json:"targets"`
}

func BuildChangeContract(snapshot Snapshot, analysis Analysis, requests []ChangeRequest) (ChangeContract, error) {
	if err := snapshot.Validate(); err != nil {
		return ChangeContract{}, fmt.Errorf("repository change contract snapshot: %w", err)
	}
	if err := analysis.Validate(snapshot); err != nil {
		return ChangeContract{}, fmt.Errorf("repository change contract analysis: %w", err)
	}
	if !analysis.Complete {
		return ChangeContract{}, fmt.Errorf("repository change contract requires a complete analysis")
	}
	if len(requests) == 0 || len(requests) > maxChangeTargets {
		return ChangeContract{}, fmt.Errorf("repository change contract requires 1-%d targets", maxChangeTargets)
	}
	symbols := make(map[string]Symbol, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	seen := make(map[string]struct{}, len(requests))
	contract := ChangeContract{
		Schema: ChangeContractSchemaV2, SnapshotID: snapshot.ID, AnalysisID: analysis.ID,
		Targets: make([]ChangeTarget, 0, len(requests)),
	}
	for _, request := range requests {
		if _, duplicate := seen[request.SymbolID]; duplicate {
			return ChangeContract{}, fmt.Errorf("repository change contract target %q is duplicated", request.SymbolID)
		}
		seen[request.SymbolID] = struct{}{}
		symbol, exists := symbols[request.SymbolID]
		if !exists {
			return ChangeContract{}, fmt.Errorf("repository change contract target %q is absent from the analysis", request.SymbolID)
		}
		if symbol.Kind != "function" && symbol.Kind != "method" {
			return ChangeContract{}, fmt.Errorf("repository change contract target %q has unsupported kind %q", symbol.ID, symbol.Kind)
		}
		if symbol.Signature == "" || symbol.Signature != strings.TrimSpace(symbol.Signature) ||
			strings.ContainsAny(symbol.Signature, "\r\n") || len([]byte(symbol.Signature)) > 1024 {
			return ChangeContract{}, fmt.Errorf("repository change contract target %q has an invalid bounded signature", symbol.ID)
		}
		if request.RequirementQuote == "" || !utf8.ValidString(request.RequirementQuote) ||
			strings.ContainsRune(request.RequirementQuote, '\x00') ||
			request.RequirementQuote != strings.TrimSpace(request.RequirementQuote) ||
			len([]byte(request.RequirementQuote)) > maxChangeRequirementBytes {
			return ChangeContract{}, fmt.Errorf(
				"repository change contract target %q requires one valid UTF-8, NUL-free, trimmed requirement quote of at most %d bytes",
				symbol.ID, maxChangeRequirementBytes,
			)
		}
		span, err := ReadExactSymbolSpan(snapshot, symbol, maxChangeSymbolBytes)
		if err != nil {
			return ChangeContract{}, err
		}
		capabilities, err := directChangeCapabilities(symbol.ID, analysis, symbols)
		if err != nil {
			return ChangeContract{}, err
		}
		verificationSymbols, err := directChangeTests(symbol.ID, analysis, symbols)
		if err != nil {
			return ChangeContract{}, err
		}
		target := ChangeTarget{
			SymbolID: symbol.ID, FileID: symbol.FileID, Kind: symbol.Kind,
			Signature: symbol.Signature, StartByte: symbol.StartByte, EndByte: symbol.EndByte,
			ExpectedFileSHA256:        symbol.SourceSHA256,
			ExpectedDeclarationSHA256: changeDigest([]byte(span.Content)),
			RequirementQuote:          request.RequirementQuote,
			DirectCapabilities:        capabilities,
			VerificationSymbolIDs:     verificationSymbols,
		}
		contract.Targets = append(contract.Targets, target)
	}
	sort.Slice(contract.Targets, func(left, right int) bool {
		return contract.Targets[left].SymbolID < contract.Targets[right].SymbolID
	})
	id, err := changeContractID(contract)
	if err != nil {
		return ChangeContract{}, err
	}
	contract.ID = id
	return contract, nil
}

func (contract ChangeContract) Validate(snapshot Snapshot, analysis Analysis) error {
	if contract.Schema != ChangeContractSchemaV2 || !validOpaqueID(contract.ID, "change_contract_") ||
		contract.SnapshotID != snapshot.ID || contract.AnalysisID != analysis.ID {
		return fmt.Errorf("repository change contract has invalid identity or source authority")
	}
	requests := make([]ChangeRequest, len(contract.Targets))
	for index, target := range contract.Targets {
		requests[index] = ChangeRequest{SymbolID: target.SymbolID, RequirementQuote: target.RequirementQuote}
	}
	expected, err := BuildChangeContract(snapshot, analysis, requests)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(contract, expected) {
		return fmt.Errorf("repository change contract differs from current exact facts")
	}
	return nil
}

func directChangeCapabilities(targetID string, analysis Analysis, symbols map[string]Symbol) ([]DirectCapability, error) {
	byID := make(map[string]DirectCapability)
	for _, edge := range analysis.Edges {
		if edge.Kind != "calls" || edge.FromID != targetID {
			continue
		}
		dependency, exists := symbols[edge.ToID]
		if !exists {
			continue
		}
		capability, err := newDirectCapability(dependency)
		if err != nil {
			return nil, err
		}
		byID[dependency.ID] = capability
	}
	target, exists := symbols[targetID]
	if !exists {
		return nil, fmt.Errorf("repository change target %q is absent while deriving capabilities", targetID)
	}
	typeDependencies, err := directSignatureTypeDependencies(target, symbols)
	if err != nil {
		return nil, err
	}
	for _, dependency := range typeDependencies {
		capability, capabilityErr := newDirectCapability(dependency)
		if capabilityErr != nil {
			return nil, capabilityErr
		}
		byID[dependency.ID] = capability
	}
	result := make([]DirectCapability, 0, len(byID))
	for _, capability := range byID {
		result = append(result, capability)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].SymbolID < result[right].SymbolID })
	capabilityBytes, symbolBytes := 0, 0
	for _, capability := range result {
		capabilityBytes += len([]byte(capability.Signature))
		for _, symbol := range capability.PermittedSymbols {
			symbolBytes += len([]byte(symbol))
		}
	}
	if len(result) > maxDirectCapabilities || capabilityBytes > maxDirectCapabilityBytes ||
		symbolBytes > maxPermittedSymbolBytes {
		return nil, fmt.Errorf(
			"repository change target %q direct capabilities exceed the %d-item, %d-signature-byte, or %d-symbol-byte boundary",
			targetID, maxDirectCapabilities, maxDirectCapabilityBytes, maxPermittedSymbolBytes,
		)
	}
	return result, nil
}

func newDirectCapability(symbol Symbol) (DirectCapability, error) {
	permitted := []string{symbol.Name}
	if symbol.Kind == "type" {
		permitted = goDeclarationIdentifiers(symbol.Signature)
	}
	permitted = canonicalStrings(permitted)
	if len(permitted) == 0 {
		return DirectCapability{}, fmt.Errorf("repository direct capability %q has no permitted symbols", symbol.ID)
	}
	return DirectCapability{
		SymbolID: symbol.ID, Name: symbol.Name, Signature: symbol.Signature,
		SourceSHA256: symbol.SourceSHA256, PermittedSymbols: permitted,
	}, nil
}

func directSignatureTypeDependencies(target Symbol, symbols map[string]Symbol) ([]Symbol, error) {
	file, err := parser.ParseFile(
		token.NewFileSet(), "", "package capability\n\n"+target.Signature+" {}", parser.AllErrors,
	)
	if err != nil {
		return nil, fmt.Errorf("parse repository target %q signature dependencies: %w", target.ID, err)
	}
	if len(file.Decls) != 1 {
		return nil, fmt.Errorf("repository target %q signature produced %d declarations", target.ID, len(file.Decls))
	}
	function, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok {
		return nil, fmt.Errorf("repository target %q signature is not a function declaration", target.ID)
	}
	referenced := make(map[string]struct{})
	nodes := []ast.Node{function.Type}
	if function.Recv != nil {
		nodes = append(nodes, function.Recv)
	}
	for _, node := range nodes {
		ast.Inspect(node, func(current ast.Node) bool {
			if identifier, isIdentifier := current.(*ast.Ident); isIdentifier {
				referenced[identifier.Name] = struct{}{}
			}
			return true
		})
	}
	packageName := qualifiedSymbolPackage(target.QualifiedName)
	byName := make(map[string]Symbol)
	for _, symbol := range symbols {
		if symbol.Kind != "type" || qualifiedSymbolPackage(symbol.QualifiedName) != packageName {
			continue
		}
		if _, used := referenced[symbol.Name]; !used {
			continue
		}
		if prior, duplicate := byName[symbol.Name]; duplicate && prior.ID != symbol.ID {
			return nil, fmt.Errorf(
				"repository target %q has ambiguous direct type dependency %q", target.ID, symbol.Name,
			)
		}
		byName[symbol.Name] = symbol
	}
	dependencies := make([]Symbol, 0, len(byName))
	for _, symbol := range byName {
		dependencies = append(dependencies, symbol)
	}
	sort.Slice(dependencies, func(left, right int) bool { return dependencies[left].ID < dependencies[right].ID })
	return dependencies, nil
}

func qualifiedSymbolPackage(qualified string) string {
	if index := strings.LastIndex(qualified, "."); index >= 0 {
		return qualified[:index]
	}
	return ""
}

func goDeclarationIdentifiers(source string) []string {
	fileSet := token.NewFileSet()
	file := fileSet.AddFile("", fileSet.Base(), len(source))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(source), nil, 0)
	identifiers := make([]string, 0, 8)
	for {
		_, current, literal := lexer.Scan()
		if current == token.EOF {
			return identifiers
		}
		if current == token.IDENT {
			identifiers = append(identifiers, literal)
		}
	}
}

func canonicalStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (target ChangeTarget) PermittedCapabilitySymbols() []string {
	values := make([]string, 0)
	for _, capability := range target.DirectCapabilities {
		values = append(values, capability.PermittedSymbols...)
	}
	return canonicalStrings(values)
}

func directChangeTests(targetID string, analysis Analysis, symbols map[string]Symbol) ([]string, error) {
	set := make(map[string]struct{})
	for _, edge := range analysis.Edges {
		if edge.Kind == "tests" && edge.ToID == targetID {
			if test, exists := symbols[edge.FromID]; exists && test.Kind == "test" {
				set[test.ID] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	if len(result) > maxVerificationSymbols {
		return nil, fmt.Errorf(
			"repository change target %q has %d direct verification symbols and exceeds %d",
			targetID, len(result), maxVerificationSymbols,
		)
	}
	return result, nil
}

func changeContractID(contract ChangeContract) (string, error) {
	contract.ID = ""
	raw, err := json.Marshal(contract)
	if err != nil {
		return "", fmt.Errorf("encode repository change contract identity: %w", err)
	}
	return "change_contract_" + changeDigest(raw), nil
}

func changeDigest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
