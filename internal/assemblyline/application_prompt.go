package assemblyline

import (
	"strings"
)

func BuildApplicationClassificationPrompt(input ApplicationClassificationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the observable delivery surface of one software request.",
		"Choose browser_application when the user expects an interactive browser page, command_line_application for a terminal program, service_application for a server/API without a requested browser interface, or unsupported when none applies. Do not identify the product, extract features, infer architecture, name files, choose a framework or language, or implement anything.",
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}

func BuildApplicationIdentityPrompt(input ApplicationIdentityInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Copy only the shortest exact contiguous quote that names the requested product or application category.",
		"The quote must identify what is being built. Exclude requested features, quality or scope wording, and request phrasing. Never paraphrase, classify the delivery surface, design, or implement anything.",
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}

func BuildRequirementPartitionPrompt(input RequirementPartitionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	instruction := "Extract every explicit requested application feature from USER_REQUEST as the shortest exact contiguous quotes that each name one different feature. A feature may be phrased only as a noun. Preserve meaningful modifiers and copy source text exactly; never paraphrase. Exclude request wording, product/category naming, project scope or quality wording, constraints, and connective filler. Return an empty feature_quotes array only when no application feature is requested. Return feature quotes in source order. Do not classify kinds, write outcomes, design, or implement anything."
	label := "USER_REQUEST:\n"
	if input.Mode == RequirementSplitFeature {
		instruction = "Split FEATURE_ENVELOPE into the shortest exact contiguous quotes that each name one different requested application feature. The envelope is already known to contain feature work; do not reclassify it as product or project context. Preserve meaningful modifiers and copy source text exactly; never paraphrase. If it already names exactly one feature, return it unchanged as the sole item. Return at least one feature quote in source order. Do not classify kinds, write outcomes, design, or implement anything."
		label = "FEATURE_ENVELOPE:\n"
	}
	return strings.Join([]string{
		instruction,
		label + input.SourceText,
	}, "\n\n"), nil
}

func BuildArtifactHandlingPrompt(input ArtifactHandlingInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the user's explicit authority over FOCUSED_ARTIFACT.",
		"Choose preserve_unchanged when it must not be modified, must_exist when its existence is required but its contents are not authorized to change, or mentioned_only when it is merely referenced. Do not infer its identity, contents, path, or implementation role.",
		"FOCUSED_ARTIFACT: " + input.Token,
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}
