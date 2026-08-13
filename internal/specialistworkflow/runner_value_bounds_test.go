package specialistworkflow_test

import "errors"

func validateRunnerValues(values []string) error {
	if len(values) > 4 {
		return errors.New("runner values exceed four entries")
	}
	for _, value := range values {
		if len(value) > 128 {
			return errors.New("runner value exceeds 128 bytes")
		}
	}
	return nil
}
