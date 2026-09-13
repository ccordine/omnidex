package assemblyline

type ApplicationInputSource string

const (
	WorkApplicationInputSource    WorkKind               = "application_input_source"
	ApplicationInputArguments     ApplicationInputSource = "arguments"
	ApplicationInputStandardInput ApplicationInputSource = "standard_input"
	ApplicationInputBoth          ApplicationInputSource = "both"
	ApplicationInputNone          ApplicationInputSource = "none"
)

type ApplicationInputSourceInput struct {
	Requirement string `json:"requirement"`
}

func (input ApplicationInputSourceInput) validate() error {
	return validateRequirementQuote("input source", input.Requirement)
}

func NewApplicationInputSourceJob(input ApplicationInputSourceInput) (PortableJob, error) {
	if err := input.validate(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkApplicationInputSource, input)
}

func applicationInputSourceChoices() ([]OpaqueModelChoice, error) {
	var choices []OpaqueModelChoice
	for _, item := range []struct {
		description string
		kind        ApplicationInputSource
	}{
		{"Command-line argument values.", ApplicationInputArguments},
		{"Text read from standard input.", ApplicationInputStandardInput},
		{"Both command-line argument values and standard-input text.", ApplicationInputBoth},
		{"No command-line or standard-input value is required.", ApplicationInputNone},
	} {
		choice, err := NewOpaqueModelChoice(item.description, string(item.kind))
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	return choices, nil
}

func BuildApplicationInputSourcePrompt(input ApplicationInputSourceInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := applicationInputSourceChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion("Which process input does this behavior require?", []string{"Required behavior:\n" + input.Requirement}, choices)
}

func DecodeApplicationInputSource(input ApplicationInputSourceInput, raw string) (ApplicationInputSource, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := applicationInputSourceChoices()
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	return ApplicationInputSource(value), err
}
