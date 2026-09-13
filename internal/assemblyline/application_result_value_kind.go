package assemblyline

type ApplicationResultValueKind string

const (
	WorkApplicationResultValueKind WorkKind                   = "application_result_value_kind"
	ApplicationResultText          ApplicationResultValueKind = "text"
	ApplicationResultInteger       ApplicationResultValueKind = "integer"
	ApplicationResultDecimal       ApplicationResultValueKind = "decimal"
	ApplicationResultBoolean       ApplicationResultValueKind = "boolean"
	ApplicationResultOther         ApplicationResultValueKind = "other"
)

type ApplicationResultValueKindInput struct {
	Requirement string `json:"requirement"`
}

func (input ApplicationResultValueKindInput) validate() error {
	return validateRequirementQuote("result value kind", input.Requirement)
}

func NewApplicationResultValueKindJob(input ApplicationResultValueKindInput) (PortableJob, error) {
	if err := input.validate(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkApplicationResultValueKind, input)
}

func applicationResultValueKindChoices() ([]OpaqueModelChoice, error) {
	var choices []OpaqueModelChoice
	for _, item := range []struct {
		description string
		kind        ApplicationResultValueKind
	}{
		{"Text, whose wording is the requested result.", ApplicationResultText},
		{"A whole-number value.", ApplicationResultInteger},
		{"A numerical value that can have a fractional part.", ApplicationResultDecimal},
		{"A Boolean truth value.", ApplicationResultBoolean},
		{"A result that cannot be represented by one of these value kinds.", ApplicationResultOther},
	} {
		choice, err := NewOpaqueModelChoice(item.description, string(item.kind))
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	return choices, nil
}

func BuildApplicationResultValueKindPrompt(input ApplicationResultValueKindInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := applicationResultValueKindChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion("What kind of value does this behavior produce, before presentation?", []string{"Required behavior:\n" + input.Requirement}, choices)
}

func DecodeApplicationResultValueKind(input ApplicationResultValueKindInput, raw string) (ApplicationResultValueKind, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := applicationResultValueKindChoices()
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	return ApplicationResultValueKind(value), err
}
