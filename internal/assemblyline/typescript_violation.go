package assemblyline

type TypeScriptFragmentViolationCode string

const (
	TypeScriptViolationEmptyBody TypeScriptFragmentViolationCode = "empty_executable_body"
)

type TypeScriptFragmentViolation struct {
	Code    TypeScriptFragmentViolationCode
	Message string
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
