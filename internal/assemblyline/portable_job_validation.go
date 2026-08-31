package assemblyline

import (
	"fmt"
	"regexp"
	"strings"
)

var portableWorkDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validateRequirementQuote(label, quote string) error {
	if quote == "" || quote != strings.TrimSpace(quote) {
		return fmt.Errorf("%s requires one trimmed source quote", label)
	}
	if len(quote) > maxRequirementQuoteBytes {
		return fmt.Errorf("%s source quote exceeds %d bytes", label, maxRequirementQuoteBytes)
	}
	if err := ValidatePathFreeModelContext(label, quote); err != nil {
		return err
	}
	return nil
}

func (input ApplicationClassificationInput) validate() error {
	if err := validateApplicationRequest("application classification", input.UserRequest); err != nil {
		return err
	}
	return ValidatePathFreeModelContext(
		"application classification request", input.UserRequest,
	)
}

func (input ArtifactHandlingInput) validate() error {
	if input.UserRequest == "" || input.UserRequest != strings.TrimSpace(input.UserRequest) {
		return fmt.Errorf("artifact handling requires one trimmed user request")
	}
	if !opaqueArtifactPattern.MatchString(input.Token) {
		return fmt.Errorf("artifact handling token %q is invalid", input.Token)
	}
	if !strings.Contains(input.UserRequest, input.Token) {
		return fmt.Errorf("artifact handling token %s is absent from the user request", input.Token)
	}
	return ValidatePathFreeModelContext(
		"artifact handling request", input.UserRequest,
	)
}

func validateGroundedQuoteCollection(label, source string, quotes []string) error {
	if source == "" || source != strings.TrimSpace(source) {
		return fmt.Errorf("%s quotes require one trimmed source", label)
	}
	if err := validateQuoteCollection(label, quotes); err != nil {
		return err
	}
	for index, quote := range quotes {
		if _, err := uniqueTextSpan(source, quote); err != nil {
			return fmt.Errorf("%s quote %d %q %w", label, index, quote, err)
		}
	}
	return nil
}

func validateQuoteCollection(label string, quotes []string) error {
	return validateBoundedQuoteCollection(label, quotes, maxRequirementCount)
}

func validateBoundedQuoteCollection(label string, quotes []string, limit int) error {
	if len(quotes) > limit {
		return fmt.Errorf("%s quotes exceed %d items", label, limit)
	}
	seen := make(map[string]struct{}, len(quotes))
	for index, quote := range quotes {
		if err := validateRequirementQuote(label, quote); err != nil {
			return fmt.Errorf("%s quote %d: %w", label, index, err)
		}
		if _, duplicate := seen[quote]; duplicate {
			return fmt.Errorf("%s quote %d duplicates %q", label, index, quote)
		}
		seen[quote] = struct{}{}
	}
	return nil
}

func (input FragmentGenerationInput) validate() error {
	if err := validatePortableFragmentCore(input.Language, input.Signature, input.Capabilities, input.PermittedSymbols); err != nil {
		return err
	}
	if input.Behavior == "" || input.Behavior != strings.TrimSpace(input.Behavior) {
		return fmt.Errorf("fragment generation behavior is required and must be trimmed")
	}
	if input.Dialect == "" || input.Dialect != strings.TrimSpace(input.Dialect) ||
		strings.ContainsAny(input.Dialect, "\x00\r\n") || len(input.Dialect) > 256 {
		return fmt.Errorf("fragment generation dialect is required as one bounded label")
	}
	if len(input.Behavior) > maxLocalBehaviorBytes {
		return fmt.Errorf("fragment generation behavior exceeds %d bytes", maxLocalBehaviorBytes)
	}
	return input.ValidatePathFree(ArtifactIdentityProvenance{})
}

// ValidatePathFree applies the field-scoped path boundary to the exact
// fragment envelope. Behavior is prose; signatures and capability projections
// are parser-proven source syntax.
func (input FragmentGenerationInput) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	if err := ValidatePathFreeModelContextWithProvenance(
		"fragment generation behavior", provenance, input.Dialect, input.Behavior,
	); err != nil {
		return err
	}
	sourceValues := []string{input.Signature}
	sourceValues = append(sourceValues, input.Capabilities...)
	sourceValues = append(sourceValues, input.PermittedSymbols...)
	return ValidatePathFreeSourceModelContextWithProvenance(
		"fragment generation", provenance, sourceValues...,
	)
}

func (input FragmentGenerationReplacementInput) validate() error {
	if err := input.Original.validate(); err != nil {
		return fmt.Errorf("fragment generation replacement original: %w", err)
	}
	return nil
}

func (input FragmentCorrectionInput) validate() error {
	if (input.Language == "") != (input.Signature == "") {
		return fmt.Errorf("fragment correction language and signature metadata must be both present or both absent")
	}
	if input.Language != "" {
		if err := validatePortableFragmentCore(
			input.Language, input.Signature, input.Capabilities, input.PermittedSymbols,
		); err != nil {
			return err
		}
	}
	if input.RepairGuidance == "" || input.RepairGuidance != strings.TrimSpace(input.RepairGuidance) {
		return fmt.Errorf("fragment correction requires one trimmed repair guidance instruction")
	}
	if len(input.RepairGuidance) > maxTypeScriptRepairGuidanceBytes {
		return fmt.Errorf(
			"fragment correction repair guidance exceeds %d bytes",
			maxTypeScriptRepairGuidanceBytes,
		)
	}
	if input.RequiredChange != "" || input.Diagnostic != "" {
		return fmt.Errorf(
			"fragment correction executor cannot receive a raw diagnostic or required change",
		)
	}
	if len(input.Capabilities) != 0 || len(input.PermittedSymbols) != 0 {
		return fmt.Errorf(
			"fragment correction executor cannot receive diagnostic-analysis context",
		)
	}
	current := input.CurrentDeclaration
	if (current == "") == (input.RepairRegion == nil) {
		return fmt.Errorf("fragment correction requires exactly one current declaration or repair region")
	}
	if current != "" {
		if current != strings.TrimSpace(current) {
			return fmt.Errorf("fragment correction current declaration must be trimmed")
		}
	}
	if input.RepairRegion != nil {
		if input.Language != "typescript" || input.Signature == "" {
			return fmt.Errorf("fragment correction repair regions require TypeScript")
		}
		if err := input.RepairRegion.validate(); err != nil {
			return fmt.Errorf("fragment correction repair region: %w", err)
		}
	}
	return input.ValidatePathFree(ArtifactIdentityProvenance{})
}

// ValidatePathFree preserves source grammar in declaration and repair-region
// fields, keeps diagnostics on the strict prose boundary, and preserves the
// already validated repair instruction's mixed prose/source-literal grammar.
func (input FragmentCorrectionInput) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	proseValues := []string{input.RequiredChange, input.Diagnostic}
	sourceValues := []string{input.Signature, input.CurrentDeclaration}
	sourceValues = append(sourceValues, input.Capabilities...)
	sourceValues = append(sourceValues, input.PermittedSymbols...)
	if input.RepairRegion != nil {
		sourceValues = append(sourceValues, input.RepairRegion.Source)
		for _, binding := range append(
			append([]TypeScriptRepairBinding(nil), input.RepairRegion.Bindings...),
			input.RepairRegion.UnavailableBindings...,
		) {
			sourceValues = append(sourceValues, binding.Name, binding.Type)
			sourceValues = append(sourceValues, binding.CallableSignatures...)
			sourceValues = append(sourceValues, binding.Members...)
		}
		for _, evidence := range input.RepairRegion.ExpressionEvidence {
			sourceValues = append(sourceValues, evidence.Source, evidence.InferredType, evidence.ContextualType)
			sourceValues = append(sourceValues, evidence.IncompatibleTypes...)
			sourceValues = append(sourceValues, evidence.ReferencedBindings...)
		}
	}
	if err := ValidatePathFreeModelContextWithProvenance(
		"fragment correction", provenance, proseValues...,
	); err != nil {
		return err
	}
	if err := ValidatePathFreeRepairInstructionModelContextWithProvenance(
		"fragment correction repair guidance", provenance, input.RepairGuidance,
	); err != nil {
		return err
	}
	return ValidatePathFreeSourceModelContextWithProvenance(
		"fragment correction", provenance, sourceValues...,
	)
}

func validatePortableFragmentCore(language, signature string, capabilities, symbols []string) error {
	if language == "" || language != strings.TrimSpace(language) {
		return fmt.Errorf("fragment language is required and must be trimmed")
	}
	if signature == "" || signature != strings.TrimSpace(signature) || strings.ContainsAny(signature, "\r\n") {
		return fmt.Errorf("fragment signature must be one trimmed line")
	}
	if len(signature) > 1024 {
		return fmt.Errorf("fragment signature exceeds 1024 bytes")
	}
	if err := validatePortableStringSet("capability", capabilities); err != nil {
		return err
	}
	return validateBoundedPortableStringSet("permitted symbol", symbols, 1024)
}

func validatePortableStringSet(label string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("%s %d is empty or untrimmed", label, index)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s %q is duplicated", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateBoundedPortableStringSet(label string, values []string, byteLimit int) error {
	if err := validatePortableStringSet(label, values); err != nil {
		return err
	}
	total := 0
	for _, value := range values {
		total += len(value)
	}
	if total > byteLimit {
		return fmt.Errorf("%ss exceed %d bytes", label, byteLimit)
	}
	return nil
}
