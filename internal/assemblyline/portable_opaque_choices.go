package assemblyline

import (
	"fmt"
	"strings"
)

// PortableJobOpaqueChoiceOptions projects the exact registered choice builders.
// It includes sole choices without rendering a forbidden model prompt.
func PortableJobOpaqueChoiceOptions(job PortableJob) ([]string, bool, error) {
	if err := job.Validate(); err != nil {
		return nil, false, err
	}
	contract, err := portableResponseContractForValidatedJob(job)
	if err != nil || len(contract.choices) == 0 {
		return nil, false, err
	}
	return opaqueModelChoiceLabeledDescriptions(contract.choices), true, nil
}

// PortableJobOpaqueQuestion removes only the exact formatter suffix supplied
// by the typed choices. Description bytes are never parsed as delimiters.
func PortableJobOpaqueQuestion(job PortableJob, prompt string, options []string) (string, error) {
	if err := job.Validate(); err != nil {
		return "", err
	}
	if len(options) < 2 {
		return "", fmt.Errorf("opaque question projection requires actual model-facing choices")
	}
	for index, option := range options {
		if !strings.HasPrefix(option, opaqueModelChoiceID(index)+". ") {
			return "", fmt.Errorf("opaque question projection changed an offered choice identity")
		}
	}
	suffix := "\n\n" + strings.Join(options, "\n\n") + "\n\n" + opaqueModelChoiceAnswerClause(len(options))
	prefix, matched := strings.CutSuffix(prompt, suffix)
	if !matched || prefix == "" {
		return "", fmt.Errorf("opaque question projection differs from its exact rendered choices")
	}
	if job.Kind == WorkRoleplayOngoingActionRelation {
		var input RoleplayOngoingActionRelationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", err
		}
		question := roleplayOngoingActionRelationQuestion(input)
		if prefix != question && !strings.HasSuffix(prefix, "\n\n"+question) {
			return "", fmt.Errorf("opaque roleplay question differs from its typed character authority")
		}
		return question, nil
	}
	if start := strings.LastIndex(prefix, "\n\n"); start >= 0 {
		prefix = prefix[start+2:]
	}
	if strings.TrimSpace(prefix) != prefix || prefix == "" {
		return "", fmt.Errorf("opaque question projection lacks its exact question")
	}
	return prefix, nil
}
