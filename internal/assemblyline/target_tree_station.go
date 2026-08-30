package assemblyline

import (
	"fmt"
	"strconv"
	"strings"
)

func NewTargetTreeJob(input TargetTreeInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationTargetTree, input, input.Validate)
}

func BuildTargetTreePrompt(input TargetTreeInput) (string, error) {
	if err := input.Validate(); err != nil {
		return "", err
	}
	currentTree, err := RenderTargetTree(input.ExistingPaths)
	if err != nil {
		return "", fmt.Errorf("render current managed workload tree: %w", err)
	}
	reservedTree, err := RenderTargetTree(input.ReservedPaths)
	if err != nil {
		return "", fmt.Errorf("render code-reserved tree: %w", err)
	}
	sections := []string{
		"Determine the complete expected managed workload file tree for all accepted goals under the supplied technical context.",
		"Return exactly one raw basename hierarchy matching RAW_TREE_GRAMMAR. The response is the complete expected workload tree, not a delta.",
		"RAW_TREE_GRAMMAR:\nROOT\n  D <single basename>\n    F <single basename>\n  F <single basename>",
		"ROOT must be the exact first line. Every other line uses exactly two spaces per depth followed by D or F, one space, and one basename. A basename never contains a slash or backslash. Every D node contains at least one child and ultimately one F node. An F node has no children. The entire response consists only of these hierarchy lines.",
		"EXACT_FILE_COUNT:\n" + strconv.Itoa(input.Constraints.ExactPathCount),
		"ROOT_FILES_ONLY:\n" + strconv.FormatBool(input.Constraints.RootFilesOnly),
		"The response contains exactly EXACT_FILE_COUNT F nodes. When ROOT_FILES_ONLY is true, it contains no D nodes. Every file leaf and the complete tree must satisfy TECHNICAL_CONTEXT exactly. A node present in RESERVED_TREE cannot appear in the response.",
		"ACCEPTED_GOALS:\n" + input.Objective,
		"TECHNICAL_CONTEXT:\n" + input.TechnicalContext,
		"CURRENT_MANAGED_WORKLOAD_TREE:\n" + currentTree,
		"RESERVED_TREE:\n" + reservedTree,
	}
	if correction := input.Correction; correction != nil {
		if correction.CandidateTree != "" {
			sections = append(sections, "CURRENT_SAFE_TREE_CANDIDATE:\n"+correction.CandidateTree)
		}
		sections = append(sections,
			"VALIDATION_FAILURE:\n"+correction.Failure,
			"Return one complete replacement basename hierarchy matching RAW_TREE_GRAMMAR that resolves this exact validation failure.",
		)
	}
	prompt := strings.Join(sections, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("target tree prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	if err := ValidatePathFreeModelContext("target tree rendered prompt", prompt); err != nil {
		return "", err
	}
	return prompt, nil
}

func DecodeTargetTreeCandidate(input TargetTreeInput, raw string) (TargetTree, error) {
	var zero TargetTree
	if err := input.Validate(); err != nil {
		return zero, err
	}
	target, err := ParseTargetTree(raw)
	if err != nil {
		return zero, fmt.Errorf("decode raw target tree: %w", err)
	}
	if err := ValidateTargetTreeConstraints(input.Constraints, target); err != nil {
		return zero, err
	}
	if err := ValidateTargetTreeExistingDirectories(input.ExistingDirs, target); err != nil {
		return zero, err
	}
	if err := ValidateTargetTreeReservedPaths(input.ReservedPaths, target); err != nil {
		return zero, err
	}
	return target, nil
}
