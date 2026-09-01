package assemblyline

type TypeScriptFragmentViolationCode string

const (
	TypeScriptViolationEmptyBody           TypeScriptFragmentViolationCode = "empty_executable_body"
	TypeScriptViolationForbiddenIdentifier TypeScriptFragmentViolationCode = "forbidden_identifier"
)

type TypeScriptFragmentViolation struct {
	Code      TypeScriptFragmentViolationCode
	Message   string
	StartByte int
	EndByte   int
}

func (violation *TypeScriptFragmentViolation) Error() string {
	if violation == nil {
		return "TypeScript fragment violation is nil"
	}
	return violation.Message
}

func newTypeScriptFragmentViolation(
	code TypeScriptFragmentViolationCode,
	message string,
) error {
	return &TypeScriptFragmentViolation{Code: code, Message: message}
}

func newLocatedTypeScriptFragmentViolation(
	code TypeScriptFragmentViolationCode,
	message string,
	startByte int,
	endByte int,
) error {
	return &TypeScriptFragmentViolation{
		Code: code, Message: message, StartByte: startByte, EndByte: endByte,
	}
}
