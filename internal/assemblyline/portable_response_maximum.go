package assemblyline

import (
	"errors"
	"fmt"
)

var errPortableResponseMaximumKindMissing = errors.New(
	"portable work kind has no registered response maximum",
)

// PortableResponseMaximumBytesForJob returns the exact largest raw provider
// candidate that the validated job's registered decoder can accept. Dynamic
// candidate sets and correction inheritance are resolved from immutable job
// payload authority. Zero means the registered ordinary-text decoder imposes
// no semantic byte ceiling; it is not a replacement larger fixed limit.
func PortableResponseMaximumBytesForJob(job PortableJob) (int, error) {
	contract, err := portableResponseContractForValidatedJob(job)
	maximum := contract.maximum
	if err != nil {
		return 0, err
	}
	if contract.openText {
		return 0, nil
	}
	if maximum < 1 || maximum > MaxPortableRawCandidateBytes {
		return 0, fmt.Errorf(
			"portable work kind %q produced invalid response maximum %d",
			job.Kind, maximum,
		)
	}
	return maximum, nil
}

func portableResponseContractForValidatedJob(job PortableJob) (portableResponseContract, error) {
	for _, resolve := range []func(PortableJob) (portableResponseContract, bool, error){
		portableApplicationResponseMaximum,
		portableRepositoryConversationResponseMaximum,
		portableDatabaseResponseMaximum,
		portableWebResponseMaximum,
		portableCodingResponseMaximum,
	} {
		maximum, handled, err := resolve(job)
		if handled {
			return maximum, err
		}
	}
	return portableResponseContract{}, fmt.Errorf("%w: %q", errPortableResponseMaximumKindMissing, job.Kind)
}

func maximumStringBytes[T ~string](values ...T) int {
	maximum := 0
	for _, value := range values {
		if len(value) > maximum {
			maximum = len(value)
		}
	}
	return maximum
}

func cappedResponseMaximum(maximum, ceiling int) int {
	if maximum < ceiling {
		return maximum
	}
	return ceiling
}
