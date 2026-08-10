package labyrinth

import "fmt"

func GenerateExtended(config ExtendedGeneratorConfig) (ExtendedCase, error) {
	if err := config.Validate(); err != nil {
		return ExtendedCase{}, err
	}
	var (
		generated ExtendedCase
		err       error
	)
	switch config.Suite {
	case SuiteTraverse:
		generated, err = generateTraverse(config)
	case SuiteBind:
		generated, err = generateBind(config)
	case SuiteRevise:
		generated, err = generateRevise(config)
	case SuiteOrder:
		generated, err = generateOrder(config)
	case SuiteRogue:
		generated, err = generateRogue(config)
	default:
		return ExtendedCase{}, fmt.Errorf("%w: extended suite is not registered", ErrInvalidGeneratorConfig)
	}
	if err != nil {
		return ExtendedCase{}, fmt.Errorf("%w: generate %s: %v", ErrGeneration, config.Suite, err)
	}
	return generated, generated.Validate()
}
