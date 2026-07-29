package assemblyline

import (
	"fmt"
	"strings"
)

func (input RequirementPartitionInput) validate() error {
	switch input.Mode {
	case RequirementExtractFeatures:
		return validateResidualText("requirement feature extraction", input.SourceText)
	case RequirementSplitFeature:
		return validateRequirementQuote("requirement feature split", input.SourceText)
	default:
		return fmt.Errorf("requirement partition mode %q is unsupported", input.Mode)
	}
}

func validateRequirementQuote(label, quote string) error {
	if quote == "" || quote != strings.TrimSpace(quote) {
		return fmt.Errorf("%s requires one trimmed source quote", label)
	}
	if len(quote) > maxRequirementQuoteBytes {
		return fmt.Errorf("%s source quote exceeds %d bytes", label, maxRequirementQuoteBytes)
	}
	return nil
}

func validateResidualText(label, residual string) error {
	if strings.TrimSpace(residual) == "" {
		return fmt.Errorf("%s requires unresolved source text", label)
	}
	if len(residual) > maxPortablePayloadBytes/2 {
		return fmt.Errorf("%s residual text exceeds %d bytes", label, maxPortablePayloadBytes/2)
	}
	return nil
}

func (input ApplicationClassificationInput) validate() error {
	if input.UserRequest == "" || input.UserRequest != strings.TrimSpace(input.UserRequest) {
		return fmt.Errorf("application classification requires one trimmed user request")
	}
	return nil
}

func (input ApplicationIdentityInput) validate() error {
	if input.UserRequest == "" || input.UserRequest != strings.TrimSpace(input.UserRequest) {
		return fmt.Errorf("application identity requires one trimmed user request")
	}
	return nil
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
	return nil
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
	if len(quotes) > maxRequirementCount {
		return fmt.Errorf("%s quotes exceed %d items", label, maxRequirementCount)
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
	if len(input.Behavior) > maxLocalBehaviorBytes {
		return fmt.Errorf("fragment generation behavior exceeds %d bytes", maxLocalBehaviorBytes)
	}
	return nil
}

func (input FragmentCorrectionInput) validate() error {
	if err := validatePortableFragmentCore(input.Language, input.Signature, input.Capabilities, input.PermittedSymbols); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"current declaration": input.CurrentDeclaration,
		"required change":     input.RequiredChange,
		"diagnostic":          input.Diagnostic,
	} {
		if value == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("fragment correction %s is required and must be trimmed", label)
		}
	}
	if len(input.CurrentDeclaration) > maxTypeScriptCurrentDeclarationBytes {
		return fmt.Errorf("fragment correction current declaration exceeds %d bytes", maxTypeScriptCurrentDeclarationBytes)
	}
	if len(input.RequiredChange) > maxTypeScriptRequiredChangeBytes {
		return fmt.Errorf("fragment correction required change exceeds %d bytes", maxTypeScriptRequiredChangeBytes)
	}
	if len(input.Diagnostic) > maxTypeScriptDiagnosticBytes {
		return fmt.Errorf("fragment correction diagnostic exceeds %d bytes", maxTypeScriptDiagnosticBytes)
	}
	return nil
}

func (input ResponseCorrectionInput) validate() error {
	if err := input.Original.Validate(); err != nil {
		return fmt.Errorf("response correction original job: %w", err)
	}
	if input.Original.Kind == WorkResponseCorrection {
		return fmt.Errorf("response correction cannot wrap another response correction")
	}
	if input.ValidationFailure == "" || input.ValidationFailure != strings.TrimSpace(input.ValidationFailure) {
		return fmt.Errorf("response correction requires one trimmed validation failure")
	}
	if len(input.ValidationFailure) > 1200 {
		return fmt.Errorf("response correction validation failure exceeds 1200 bytes")
	}
	_, err := responseCorrectionSchema(input.Original)
	return err
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
	if err := validatePortableStringSet("capability", capabilities, 2048); err != nil {
		return err
	}
	return validatePortableStringSet("permitted symbol", symbols, 1024)
}

func validatePortableStringSet(label string, values []string, byteLimit int) error {
	seen := make(map[string]struct{}, len(values))
	total := 0
	for index, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("%s %d is empty or untrimmed", label, index)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s %q is duplicated", label, value)
		}
		seen[value] = struct{}{}
		total += len(value)
	}
	if total > byteLimit {
		return fmt.Errorf("%ss exceed %d bytes", label, byteLimit)
	}
	return nil
}
