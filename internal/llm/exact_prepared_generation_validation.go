package llm

import (
	"fmt"
	"strings"
)

// ValidateExactPreparedGenerationForRequest binds a successful provider
// generation to the exact request that produced it. Generic receipt validation
// cannot decide whether provider-profile control bytes leaked into content.
func ValidateExactPreparedGenerationForRequest(
	prepared PreparedModel,
	generation PreparedGeneration,
) error {
	if err := validateExactPreparedRequest(prepared); err != nil {
		return fmt.Errorf("validate exact prepared generation request: %w", err)
	}
	if err := generation.Validate(); err != nil {
		return err
	}
	requestSHA256, err := ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return err
	}
	if generation.ProviderRequestSHA256 != requestSHA256 ||
		generation.ProviderResponseModel != prepared.ContextModel {
		return fmt.Errorf("exact prepared generation differs from its request authority")
	}
	profile, err := exactProviderModelProfileByID(
		prepared.ProviderIdentityExpectation.TokenizerProfile,
	)
	if err != nil {
		return err
	}
	if prepared.OutputLimitMode == ExactPreparedOutputLimitNatural &&
		profile.naturalOutputCeiling {
		if err := ValidateExactPreparedNaturalUsageWithOutputCeiling(
			prepared.ContextTokens, prepared.MaxOutputTokens, generation.Usage,
		); err != nil {
			return err
		}
	}
	if profile.transport != exactPreparedTransportRaw {
		return nil
	}
	if generation.Thinking != "" {
		return fmt.Errorf("exact raw prepared generation leaked separate thinking content")
	}
	for _, control := range []string{
		"<|im_start|>",
		ExactPreparedRawChatEndV1,
	} {
		if strings.Contains(generation.Content, control) {
			return fmt.Errorf("exact raw prepared generation leaked a reserved ChatML control")
		}
	}
	return nil
}
