package queue

const cognitionObligationCommandSchemaV1 = "omnidex.cognition-obligation-command.v1"

type cognitionObligationDescriptor struct {
	ID     string
	SHA256 string
	Kind   CognitionObligationCommandKind
	Raw    []byte
}
