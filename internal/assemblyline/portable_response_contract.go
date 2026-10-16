package assemblyline

// The registered decoder's grammar and actual choices share one resolver.
// Choice values remain internal; public observations receive descriptions only.
type portableResponseContract struct {
	maximum  int
	openText bool
	choices  []OpaqueModelChoice
}

func opaqueModelChoiceResponseContract(choices []OpaqueModelChoice) (portableResponseContract, error) {
	maximum, err := opaqueModelChoiceResponseMaximum(choices)
	if err != nil {
		return portableResponseContract{}, err
	}
	return portableResponseContract{maximum: maximum, choices: choices}, nil
}

func opaqueModelChoiceBuilderResponseContract(build func() ([]OpaqueModelChoice, error)) (portableResponseContract, error) {
	choices, err := build()
	if err != nil {
		return portableResponseContract{}, err
	}
	return opaqueModelChoiceResponseContract(choices)
}
