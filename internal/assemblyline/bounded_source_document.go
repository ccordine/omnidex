package assemblyline

import (
	"fmt"
	"strings"
	"unsafe"
)

func composeBoundedSourceDocument(
	language boundedSourceLanguage,
	document SourceDocument,
	composition SourceComposition,
) (ComposedSourceDocument, error) {
	if composition.Generated == nil || composition.Interfaces == nil {
		return ComposedSourceDocument{}, fmt.Errorf(
			"%s composition requires generated source and interface maps", language.display,
		)
	}
	if err := validateBoundedSourceDocument(language, document); err != nil {
		return ComposedSourceDocument{}, err
	}

	var source strings.Builder
	line := 1
	preamble := composeSourceDocumentPreamble(document)
	if preamble != "" {
		source.WriteString(preamble)
		source.WriteString("\n\n")
		line += strings.Count(preamble, "\n") + 2
	}
	spans := make(map[string]SourceSpan, len(document.Blocks))
	for index, block := range document.Blocks {
		declaration := strings.TrimSpace(block.Static)
		if block.Generated() {
			candidate, exists := composition.Generated[block.ID]
			if !exists || strings.TrimSpace(candidate) == "" {
				return ComposedSourceDocument{}, fmt.Errorf("generated block %s has no source", block.ID)
			}
			if err := validateBoundedSourceCapabilities(block, composition.Interfaces); err != nil {
				return ComposedSourceDocument{}, err
			}
			var err error
			declaration, err = validateBoundedSourceFragment(language, block.Signature, candidate)
			if err != nil {
				return ComposedSourceDocument{}, fmt.Errorf("generated block %s: %w", block.ID, err)
			}
			if block.Export {
				if !language.allowCodeOwnedExport {
					return ComposedSourceDocument{}, fmt.Errorf(
						"%s generated block %s cannot use code-owned export authority",
						language.display, block.ID,
					)
				}
				if strings.HasPrefix(strings.TrimSpace(block.Signature), "export ") {
					return ComposedSourceDocument{}, fmt.Errorf(
						"%s generated block %s repeats export authority in its signature",
						language.display, block.ID,
					)
				}
				declaration = "export " + declaration
			}
		}

		start := line
		source.WriteString(declaration)
		source.WriteString("\n")
		end := line + strings.Count(declaration, "\n")
		spans[block.ID] = SourceSpan{StartLine: start, EndLine: end}
		line = end + 1
		if index < len(document.Blocks)-1 {
			source.WriteString("\n")
			line++
		}
	}
	if postamble := strings.TrimSpace(document.Postamble); postamble != "" {
		source.WriteString("\n")
		source.WriteString(postamble)
		source.WriteString("\n")
	}
	assembled := source.String()
	if err := validateBoundedSourceSyntax(language, language.documentLanguage, assembled); err != nil {
		return ComposedSourceDocument{}, fmt.Errorf(
			"parse assembled %s document %s: %w", language.display, document.ID, err,
		)
	}
	return ComposedSourceDocument{
		ID: document.ID, Path: document.Path, Source: assembled, Spans: spans,
	}, nil
}

func validateBoundedSourceBlueprint(
	language boundedSourceLanguage,
	blueprint SourceBlueprint,
) error {
	if err := blueprint.Validate(); err != nil {
		return err
	}
	for _, document := range blueprint.Documents {
		if err := validateBoundedSourceDocument(language, document); err != nil {
			return err
		}
	}
	return nil
}

func validateBoundedSourceDocument(
	language boundedSourceLanguage,
	document SourceDocument,
) error {
	if !language.pathAllowed(document.Path) {
		return fmt.Errorf(
			"document %s path %q must be %s source", document.ID, document.Path, language.display,
		)
	}
	if len(document.Blocks) == 0 {
		return fmt.Errorf("%s document %s requires at least one source block", language.display, document.ID)
	}
	preamble := composeSourceDocumentPreamble(document)
	if language.id == "java" {
		if err := validateJavaSourceDocumentWrapper(language, document); err != nil {
			return err
		}
	} else if strings.TrimSpace(document.Postamble) != "" {
		return fmt.Errorf(
			"%s document %s does not support a postamble", language.display, document.ID,
		)
	}
	if language.requirePHPOpeningTag {
		trimmed := strings.TrimSpace(preamble)
		hasOpeningTag := trimmed == "<?php" || strings.HasPrefix(trimmed, "<?php ") ||
			strings.HasPrefix(trimmed, "<?php\t") || strings.HasPrefix(trimmed, "<?php\n") ||
			strings.HasPrefix(trimmed, "<?php\r")
		if !hasOpeningTag || strings.Count(trimmed, "<?php") != 1 || strings.Contains(trimmed, "?>") {
			return fmt.Errorf("PHP document %s requires one unclosed <?php preamble", document.ID)
		}
	}
	if preamble != "" && language.id != "java" {
		if err := validateBoundedSourceSyntax(language, language.documentLanguage, preamble+"\n"); err != nil {
			return fmt.Errorf("parse %s document %s preamble: %w", language.display, document.ID, err)
		}
	}
	for index, block := range document.Blocks {
		if err := validateSourceBlock(block); err != nil {
			return fmt.Errorf("document %s block %d: %w", document.ID, index, err)
		}
		if !sourceFunctionPolicyIsEmpty(block.Policy) {
			return fmt.Errorf(
				"document %s block %s: %s source policy is unsupported by the registered parser",
				document.ID, block.ID, language.display,
			)
		}
		if block.Export && (!language.allowCodeOwnedExport || !block.Generated()) {
			return fmt.Errorf(
				"document %s block %s: code-owned export authority is unsupported for this block",
				document.ID, block.ID,
			)
		}
		if block.Generated() {
			if _, err := validateBoundedSourceFragment(language, block.Signature, block.Signature+" {}"); err != nil {
				return fmt.Errorf("document %s block %s: %w", document.ID, block.ID, err)
			}
			continue
		}
		if err := validateBoundedSourceSyntax(
			language, language.fragmentLanguage, strings.TrimSpace(block.Static),
		); err != nil {
			return fmt.Errorf("document %s static block %s: %w", document.ID, block.ID, err)
		}
	}
	return nil
}

func validateJavaSourceDocumentWrapper(
	language boundedSourceLanguage,
	document SourceDocument,
) error {
	if len(document.ScopedPreambles) != 0 {
		return fmt.Errorf("Java document %s cannot place scoped preambles inside its class wrapper", document.ID)
	}
	preamble := strings.TrimSpace(document.Preamble)
	if preamble == "" || strings.TrimSpace(document.Postamble) != "}" {
		return fmt.Errorf(
			"Java document %s requires a code-owned class preamble and one closing-brace postamble",
			document.ID,
		)
	}
	parser, tree, err := parseBoundedSourceTree(
		language, language.documentLanguage, preamble+"\n}",
	)
	if err != nil {
		return fmt.Errorf("parse Java document %s class wrapper: %w", document.ID, err)
	}
	defer parser.Close()
	defer tree.Close()
	root := tree.RootNode()
	classes := 0
	for index := uint(0); index < root.NamedChildCount(); index++ {
		declaration := root.NamedChild(index)
		if declaration == nil {
			continue
		}
		switch declaration.Kind() {
		case "package_declaration", "import_declaration":
			continue
		case "class_declaration":
			classes++
			body := declaration.ChildByFieldName("body")
			if body == nil || body.NamedChildCount() != 0 {
				return fmt.Errorf(
					"Java document %s class preamble contains member authority", document.ID,
				)
			}
		default:
			return fmt.Errorf(
				"Java document %s preamble contains unsupported %s authority",
				document.ID, declaration.Kind(),
			)
		}
	}
	if classes != 1 {
		return fmt.Errorf("Java document %s requires exactly one code-owned class wrapper", document.ID)
	}
	return nil
}

func validateBoundedSourceCapabilities(block SourceBlock, interfaces map[string]string) error {
	for _, capability := range block.Capabilities {
		if strings.TrimSpace(interfaces[capability]) == "" {
			return fmt.Errorf(
				"generated block %s capability %s has no accepted API", block.ID, capability,
			)
		}
	}
	return nil
}

func validateBoundedSourceSyntax(
	language boundedSourceLanguage,
	languagePointer func() unsafe.Pointer,
	source string,
) error {
	parser, tree, err := parseBoundedSourceTree(language, languagePointer, source)
	if err != nil {
		return err
	}
	tree.Close()
	parser.Close()
	return nil
}

func sourceFunctionPolicyIsEmpty(policy SourceFunctionPolicy) bool {
	return len(policy.RequiredCalls) == 0 && len(policy.RestrictedCalls) == 0 &&
		len(policy.TopLevelCalls) == 0 && len(policy.RequiredElementNames) == 0 &&
		len(policy.ForbiddenIdentifiers) == 0
}
