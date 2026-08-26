package assemblyline

import (
	"fmt"
	"path"
	"strings"
)

const (
	maxSourceAPIBytes = 4800
)

type SourceBlueprint struct {
	Documents []SourceDocument
}

type SourceDocument struct {
	ID              string
	Path            string
	AdapterID       string
	Preamble        string
	ScopedPreambles []SourcePreamble
	Postamble       string
	Blocks          []SourceBlock
}

type SourcePreamble struct {
	TaskID string
	Source string
}

type SourceComposition struct {
	Generated  map[string]string
	Interfaces map[string]string
}

type SourceBlockRole string

const (
	SourceBlockTaskSupport        SourceBlockRole = "task_support"
	SourceBlockTaskImplementation SourceBlockRole = "task_implementation"
	SourceBlockTaskRepresentation SourceBlockRole = "task_representation"
	SourceBlockTaskVerification   SourceBlockRole = "task_verification"
)

type SourceBlock struct {
	ID           string
	Static       string
	Signature    string
	Contract     string
	API          string
	DependsOn    []string
	Capabilities []string
	Globals      []string
	Policy       SourceFunctionPolicy
	Export       bool
	TaskID       string
	Role         SourceBlockRole
}

type SourceCallRequirement struct {
	Callees             []string
	StringArgument      string
	StringArgumentIndex int
}

type SourceFunctionPolicy struct {
	RequiredCalls        []SourceCallRequirement
	RestrictedCalls      []SourceCallRestriction
	TopLevelCalls        []string
	RequiredElementNames []string
	ForbiddenIdentifiers []string
}

type SourceCallRestriction struct {
	Callees                []string
	StringArgumentIndex    int
	AllowedStringArguments []string
}

func (b SourceBlock) Generated() bool {
	return strings.TrimSpace(b.Static) == ""
}

type SourceBlockRef struct {
	DocumentIndex int
	BlockIndex    int
	Document      SourceDocument
	Block         SourceBlock
}

func (b SourceBlueprint) Validate() error {
	if len(b.Documents) == 0 || len(b.Documents) > maxConstructionDocuments {
		return fmt.Errorf("source blueprint requires between 1 and %d documents", maxConstructionDocuments)
	}
	ids := make(map[string]struct{})
	paths := make(map[string]struct{})
	nodes := make([]DependencyNode, 0)
	for documentIndex, document := range b.Documents {
		if !graphIdentifierPattern.MatchString(document.ID) {
			return fmt.Errorf("document %d id %q is invalid", documentIndex, document.ID)
		}
		if document.AdapterID != "" && !graphIdentifierPattern.MatchString(document.AdapterID) {
			return fmt.Errorf("document %s adapter id %q is invalid", document.ID, document.AdapterID)
		}
		seenPreambles := make(map[string]struct{}, len(document.ScopedPreambles))
		for preambleIndex, preamble := range document.ScopedPreambles {
			if !graphIdentifierPattern.MatchString(preamble.TaskID) || strings.TrimSpace(preamble.Source) == "" {
				return fmt.Errorf("document %s scoped preamble %d is invalid", document.ID, preambleIndex)
			}
			if _, duplicate := seenPreambles[preamble.TaskID]; duplicate {
				return fmt.Errorf("document %s repeats scoped preamble for task %s", document.ID, preamble.TaskID)
			}
			seenPreambles[preamble.TaskID] = struct{}{}
		}
		clean := path.Clean(strings.TrimSpace(document.Path))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || clean != document.Path {
			return fmt.Errorf("document %s path %q must be normalized relative source", document.ID, document.Path)
		}
		if _, duplicate := paths[clean]; duplicate {
			return fmt.Errorf("source blueprint repeats document path %q", clean)
		}
		paths[clean] = struct{}{}
		if len(document.Blocks) == 0 {
			return fmt.Errorf("document %s requires at least one block", document.ID)
		}
		for blockIndex, block := range document.Blocks {
			if err := validateSourceBlock(block); err != nil {
				return fmt.Errorf("document %s block %d: %w", document.ID, blockIndex, err)
			}
			if _, duplicate := ids[block.ID]; duplicate {
				return fmt.Errorf("source blueprint repeats block id %q", block.ID)
			}
			ids[block.ID] = struct{}{}
			nodes = append(nodes, DependencyNode{ID: block.ID, DependsOn: block.DependsOn})
		}
	}
	_, err := BuildDependencyWaves(nodes)
	return err
}

func validateSourceBlock(block SourceBlock) error {
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
	if strings.TrimSpace(block.API) == "" || len(block.API) > maxSourceAPIBytes {
		return fmt.Errorf("block %s requires a bounded code-owned API declaration", block.ID)
	}
	if generated && (strings.ContainsAny(block.Signature, "\r\n") || len(block.Contract) > maxLocalBehaviorBytes) {
		return fmt.Errorf("generated block %s has an invalid signature or oversized contract", block.ID)
	}
	if err := validateSourceBlockTaskOwnership(block); err != nil {
		return err
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
	if err := validateSourceFunctionPolicy(block.Policy); err != nil {
		return fmt.Errorf("block %s function policy: %w", block.ID, err)
	}
	return nil
}

func validateSourceBlockTaskOwnership(block SourceBlock) error {
	if block.TaskID == "" && block.Role == "" {
		return nil
	}
	if !graphIdentifierPattern.MatchString(block.TaskID) {
		return fmt.Errorf("block %s task id %q is invalid", block.ID, block.TaskID)
	}
	switch block.Role {
	case SourceBlockTaskSupport, SourceBlockTaskImplementation,
		SourceBlockTaskRepresentation, SourceBlockTaskVerification:
	default:
		return fmt.Errorf("block %s task role %q is invalid", block.ID, block.Role)
	}
	if block.Generated() && block.Role == SourceBlockTaskSupport {
		return fmt.Errorf("generated block %s cannot use task-support role", block.ID)
	}
	if !block.Generated() && (block.Role == SourceBlockTaskImplementation ||
		block.Role == SourceBlockTaskRepresentation || block.Role == SourceBlockTaskVerification) {
		return fmt.Errorf("static block %s cannot claim generated task role %s", block.ID, block.Role)
	}
	return nil
}

func validateSourceFunctionPolicy(policy SourceFunctionPolicy) error {
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
	for _, element := range policy.RequiredElementNames {
		if !codeIdentifierPattern.MatchString(element) {
			return fmt.Errorf("required element name %q is invalid", element)
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

func (b SourceBlueprint) BuildWaves() ([][]SourceBlockRef, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	byID := make(map[string]SourceBlockRef)
	nodes := make([]DependencyNode, 0)
	for documentIndex, document := range b.Documents {
		for blockIndex, block := range document.Blocks {
			ref := SourceBlockRef{DocumentIndex: documentIndex, BlockIndex: blockIndex, Document: document, Block: block}
			byID[block.ID] = ref
			nodes = append(nodes, DependencyNode{ID: block.ID, DependsOn: block.DependsOn})
		}
	}
	idsByWave, err := BuildDependencyWaves(nodes)
	if err != nil {
		return nil, err
	}
	waves := make([][]SourceBlockRef, 0, len(idsByWave))
	for _, ids := range idsByWave {
		wave := make([]SourceBlockRef, 0, len(ids))
		for _, id := range ids {
			wave = append(wave, byID[id])
		}
		waves = append(waves, wave)
	}
	return waves, nil
}
