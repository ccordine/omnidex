package assemblyline

import (
	"fmt"
	"strings"
)

func validateRequirementQuote(label, quote string) error {
	if quote == "" || quote != strings.TrimSpace(quote) {
		return fmt.Errorf("%s requires one trimmed source quote", label)
	}
	if len(quote) > maxRequirementQuoteBytes {
		return fmt.Errorf("%s source quote exceeds %d bytes", label, maxRequirementQuoteBytes)
	}
	return nil
}

func (input ApplicationClassificationInput) validate() error {
	return validateApplicationRequest("application classification", input.UserRequest)
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
	if len(input.Behavior) > maxLocalBehaviorBytes {
		return fmt.Errorf("fragment generation behavior exceeds %d bytes", maxLocalBehaviorBytes)
	}
	return nil
}

func (input FragmentCorrectionInput) validate() error {
	if err := validatePortableFragmentCore(input.Language, input.Signature, input.Capabilities, input.PermittedSymbols); err != nil {
		return err
	}
	guided := input.RepairGuidance != ""
	diagnosticCorrection := input.RequiredChange != "" || input.Diagnostic != ""
	if guided == diagnosticCorrection {
		return fmt.Errorf(
			"fragment correction requires exactly one repair guidance or diagnostic correction authority",
		)
	}
	if guided {
		if input.Language != "typescript" {
			return fmt.Errorf("guided fragment correction requires TypeScript")
		}
		if input.RepairGuidance != strings.TrimSpace(input.RepairGuidance) {
			return fmt.Errorf("fragment correction repair guidance must be trimmed")
		}
		if len(input.RepairGuidance) > maxTypeScriptRepairGuidanceBytes {
			return fmt.Errorf(
				"fragment correction repair guidance exceeds %d bytes",
				maxTypeScriptRepairGuidanceBytes,
			)
		}
		if len(input.Capabilities) != 0 || len(input.PermittedSymbols) != 0 {
			return fmt.Errorf(
				"guided fragment correction executor cannot receive diagnostic-analysis context",
			)
		}
	} else {
		for label, value := range map[string]string{
			"required change": input.RequiredChange,
			"diagnostic":      input.Diagnostic,
		} {
			if value == "" || value != strings.TrimSpace(value) {
				return fmt.Errorf("fragment correction %s is required and must be trimmed", label)
			}
		}
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
		if input.Language != "typescript" {
			return fmt.Errorf("fragment correction repair regions require TypeScript")
		}
		if err := input.RepairRegion.validate(); err != nil {
			return fmt.Errorf("fragment correction repair region: %w", err)
		}
	}
	if !guided && len(input.RequiredChange) > maxTypeScriptRequiredChangeBytes {
		return fmt.Errorf("fragment correction required change exceeds %d bytes", maxTypeScriptRequiredChangeBytes)
	}
	if !guided && len(input.Diagnostic) > maxTypeScriptDiagnosticBytes {
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
	specializedFieldCorrection := input.Original.Kind == WorkApplicationAcceptanceGroundingReview ||
		input.Original.Kind == WorkApplicationJobSpecification
	if specializedFieldCorrection {
		if input.RetainedCandidate != "" {
			return fmt.Errorf("%s field correction cannot carry a retained candidate", input.Original.Kind)
		}
	} else {
		if input.RetainedCandidate == "" {
			return fmt.Errorf("%s response correction requires one exact retained candidate", input.Original.Kind)
		}
		if input.RetainedCandidate != strings.TrimSpace(input.RetainedCandidate) ||
			len(input.RetainedCandidate) > maxPortableCandidateBytes {
			return fmt.Errorf("retained response correction candidate is invalid or oversized")
		}
		if _, err := decodeJSONObject(input.RetainedCandidate, "retained semantic candidate"); err != nil {
			return err
		}
	}
	if specializedFieldCorrection {
		if input.TargetField == "" || input.TargetField != strings.TrimSpace(input.TargetField) {
			return fmt.Errorf("%s response correction requires one exact target field", input.Original.Kind)
		}
	} else if input.TargetField != "" {
		return fmt.Errorf("field-scoped response correction is unsupported for %s", input.Original.Kind)
	}
	_, err := responseCorrectionSchema(input.Original, input.TargetField)
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
