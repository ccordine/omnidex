package assemblyline

import (
	"fmt"
	"path"
	"strings"
)

const (
	maxTypeScriptAPIBytes = 4800
)

type TypeScriptBlueprint struct {
	Documents []TypeScriptDocument
}

type TypeScriptDocument struct {
	ID     string
	Path   string
	Header string
	Blocks []TypeScriptBlock
}

func (d TypeScriptDocument) TSX() bool {
	return strings.EqualFold(path.Ext(d.Path), ".tsx")
}

type TypeScriptBlock struct {
	ID           string
	Static       string
	Signature    string
	Contract     string
	API          string
	DependsOn    []string
	Capabilities []string
	Globals      []string
	Policy       TypeScriptFunctionPolicy
	Export       bool
}

type TypeScriptCallRequirement struct {
	Callees             []string
	StringArgument      string
	StringArgumentIndex int
}

type TypeScriptFunctionPolicy struct {
	RequiredCalls        []TypeScriptCallRequirement
	RestrictedCalls      []TypeScriptCallRestriction
	TopLevelCalls        []string
	RequiredJSXElements  []string
	ForbiddenIdentifiers []string
}

type TypeScriptCallRestriction struct {
	Callees                []string
	StringArgumentIndex    int
	AllowedStringArguments []string
}

func (b TypeScriptBlock) Generated() bool {
	return strings.TrimSpace(b.Static) == ""
}

type TypeScriptBlockRef struct {
	DocumentIndex int
	BlockIndex    int
	Document      TypeScriptDocument
	Block         TypeScriptBlock
}

func (b TypeScriptBlueprint) Validate() error {
	if len(b.Documents) == 0 || len(b.Documents) > maxConstructionDocuments {
		return fmt.Errorf("TypeScript blueprint requires between 1 and %d documents", maxConstructionDocuments)
	}
	ids := make(map[string]struct{})
	paths := make(map[string]struct{})
	nodes := make([]DependencyNode, 0)
	for documentIndex, document := range b.Documents {
		if !graphIdentifierPattern.MatchString(document.ID) {
			return fmt.Errorf("document %d id %q is invalid", documentIndex, document.ID)
		}
		clean := path.Clean(strings.TrimSpace(document.Path))
		extension := strings.ToLower(path.Ext(clean))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || clean != document.Path || extension != ".ts" && extension != ".tsx" {
			return fmt.Errorf("document %s path %q must be normalized TypeScript source", document.ID, document.Path)
		}
		if _, duplicate := paths[clean]; duplicate {
			return fmt.Errorf("TypeScript blueprint repeats document path %q", clean)
		}
		paths[clean] = struct{}{}
		if len(document.Blocks) == 0 {
			return fmt.Errorf("document %s requires at least one block", document.ID)
		}
		for blockIndex, block := range document.Blocks {
			if err := validateTypeScriptBlock(block); err != nil {
				return fmt.Errorf("document %s block %d: %w", document.ID, blockIndex, err)
			}
			if _, duplicate := ids[block.ID]; duplicate {
				return fmt.Errorf("TypeScript blueprint repeats block id %q", block.ID)
			}
			ids[block.ID] = struct{}{}
			nodes = append(nodes, DependencyNode{ID: block.ID, DependsOn: block.DependsOn})
		}
	}
	_, err := BuildDependencyWaves(nodes)
	return err
}

func validateTypeScriptBlock(block TypeScriptBlock) error {
	if !graphIdentifierPattern.MatchString(block.ID) {
		return fmt.Errorf("block id %q is invalid", block.ID)
	}
	static := strings.TrimSpace(block.Static) != ""
	hasSignature := strings.TrimSpace(block.Signature) != ""
	hasContract := strings.TrimSpace(block.Contract) != ""
	generated := hasSignature || hasContract
	if static == generated {
		return fmt.Errorf("block %s requires exactly one authority: static code or generated function", block.ID)
	}
	if generated && (!hasSignature || !hasContract) {
		return fmt.Errorf("generated block %s requires both one signature and one local behavior contract", block.ID)
	}
	if strings.TrimSpace(block.API) == "" || len(block.API) > maxTypeScriptAPIBytes {
		return fmt.Errorf("block %s requires a bounded code-owned API declaration", block.ID)
	}
	if generated && (strings.ContainsAny(block.Signature, "\r\n") || len(block.Contract) > maxLocalBehaviorBytes) {
		return fmt.Errorf("generated block %s has an invalid signature or oversized contract", block.ID)
	}
	seen := make(map[string]struct{})
	for _, dependency := range block.DependsOn {
		if !graphIdentifierPattern.MatchString(dependency) {
			return fmt.Errorf("block %s dependency %q is invalid", block.ID, dependency)
		}
		if _, duplicate := seen[dependency]; duplicate {
			return fmt.Errorf("block %s repeats dependency %s", block.ID, dependency)
		}
		seen[dependency] = struct{}{}
	}
	if !generated && len(block.Capabilities) > 0 {
		return fmt.Errorf("static block %s cannot declare model capability access", block.ID)
	}
	seenCapabilities := make(map[string]struct{})
	for _, capability := range block.Capabilities {
		if !graphIdentifierPattern.MatchString(capability) {
			return fmt.Errorf("block %s capability %q is invalid", block.ID, capability)
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return fmt.Errorf("block %s repeats capability %s", block.ID, capability)
		}
		if _, dependency := seen[capability]; !dependency {
			return fmt.Errorf("block %s capability %s is not one of its direct dependencies", block.ID, capability)
		}
		seenCapabilities[capability] = struct{}{}
	}
	seenGlobals := make(map[string]struct{})
	for _, global := range block.Globals {
		if !codeIdentifierPattern.MatchString(global) {
			return fmt.Errorf("block %s global %q is invalid", block.ID, global)
		}
		if _, duplicate := seenGlobals[global]; duplicate {
			return fmt.Errorf("block %s repeats global %s", block.ID, global)
		}
		seenGlobals[global] = struct{}{}
	}
	if err := validateTypeScriptFunctionPolicy(block.Policy); err != nil {
		return fmt.Errorf("block %s function policy: %w", block.ID, err)
	}
	return nil
}

func validateTypeScriptFunctionPolicy(policy TypeScriptFunctionPolicy) error {
	for index, requirement := range policy.RequiredCalls {
		if len(requirement.Callees) == 0 {
			return fmt.Errorf("required call %d has no allowed callee", index)
		}
		seen := make(map[string]struct{}, len(requirement.Callees))
		for _, callee := range requirement.Callees {
			parts := strings.Split(callee, ".")
			for _, part := range parts {
				if !codeIdentifierPattern.MatchString(part) {
					return fmt.Errorf("required call %d callee %q is invalid", index, callee)
				}
			}
			if _, duplicate := seen[callee]; duplicate {
				return fmt.Errorf("required call %d repeats callee %s", index, callee)
			}
			seen[callee] = struct{}{}
		}
		if requirement.StringArgument == "" && requirement.StringArgumentIndex != 0 {
			return fmt.Errorf("required call %d has an argument index without a string argument", index)
		}
		if requirement.StringArgumentIndex < 0 {
			return fmt.Errorf("required call %d has a negative argument index", index)
		}
	}
	for index, restriction := range policy.RestrictedCalls {
		if len(restriction.Callees) == 0 || len(restriction.AllowedStringArguments) == 0 {
			return fmt.Errorf("restricted call %d requires callees and allowed string arguments", index)
		}
		if restriction.StringArgumentIndex < 0 {
			return fmt.Errorf("restricted call %d has a negative argument index", index)
		}
		for _, callee := range restriction.Callees {
			for _, part := range strings.Split(callee, ".") {
				if !codeIdentifierPattern.MatchString(part) {
					return fmt.Errorf("restricted call %d callee %q is invalid", index, callee)
				}
			}
		}
		for _, argument := range restriction.AllowedStringArguments {
			if strings.TrimSpace(argument) == "" {
				return fmt.Errorf("restricted call %d has an empty allowed argument", index)
			}
		}
	}
	for _, element := range policy.RequiredJSXElements {
		if !codeIdentifierPattern.MatchString(element) {
			return fmt.Errorf("required JSX element %q is invalid", element)
		}
	}
	for _, callee := range policy.TopLevelCalls {
		if !codeIdentifierPattern.MatchString(callee) {
			return fmt.Errorf("top-level call %q is invalid", callee)
		}
	}
	for _, identifier := range policy.ForbiddenIdentifiers {
		if !codeIdentifierPattern.MatchString(identifier) {
			return fmt.Errorf("forbidden identifier %q is invalid", identifier)
		}
	}
	return nil
}

func (b TypeScriptBlueprint) BuildWaves() ([][]TypeScriptBlockRef, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	byID := make(map[string]TypeScriptBlockRef)
	nodes := make([]DependencyNode, 0)
	for documentIndex, document := range b.Documents {
		for blockIndex, block := range document.Blocks {
			ref := TypeScriptBlockRef{DocumentIndex: documentIndex, BlockIndex: blockIndex, Document: document, Block: block}
			byID[block.ID] = ref
			nodes = append(nodes, DependencyNode{ID: block.ID, DependsOn: block.DependsOn})
		}
	}
	idsByWave, err := BuildDependencyWaves(nodes)
	if err != nil {
		return nil, err
	}
	waves := make([][]TypeScriptBlockRef, 0, len(idsByWave))
	for _, ids := range idsByWave {
		wave := make([]TypeScriptBlockRef, 0, len(ids))
		for _, id := range ids {
			wave = append(wave, byID[id])
		}
		waves = append(waves, wave)
	}
	return waves, nil
}
