package assemblyline

import "errors"

type TypeScriptFragmentViolationCode string

const (
	TypeScriptViolationComment   TypeScriptFragmentViolationCode = "comment_forbidden"
	TypeScriptViolationEmptyBody TypeScriptFragmentViolationCode = "empty_executable_body"
)

type TypeScriptFragmentViolation struct {
	Code        TypeScriptFragmentViolationCode
	Message     string
	Instruction string
}

func (violation *TypeScriptFragmentViolation) Error() string {
	if violation == nil {
		return "TypeScript fragment violation is nil"
	}
	return violation.Message
}

func newTypeScriptFragmentViolation(
	code TypeScriptFragmentViolationCode,
	message, instruction string,
) error {
	return &TypeScriptFragmentViolation{Code: code, Message: message, Instruction: instruction}
}

func TypeScriptFragmentCorrectionInstruction(err error) (string, bool) {
	var violation *TypeScriptFragmentViolation
	if !errors.As(err, &violation) || violation == nil || violation.Instruction == "" {
		return "", false
	}
	return violation.Instruction, true
}
