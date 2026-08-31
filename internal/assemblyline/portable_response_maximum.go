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
// payload authority.
func PortableResponseMaximumBytesForJob(job PortableJob) (int, error) {
	maximum, err := portableResponseMaximumBytesForValidatedJob(job)
	if err != nil {
		return 0, err
	}
	if maximum < 1 || maximum > MaxPortableRawCandidateBytes {
		return 0, fmt.Errorf(
			"portable work kind %q produced invalid response maximum %d",
			job.Kind, maximum,
		)
	}
	return maximum, nil
}

func portableResponseMaximumBytesForValidatedJob(job PortableJob) (int, error) {
	for _, resolve := range []func(PortableJob) (int, bool, error){
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
	return 0, fmt.Errorf("%w: %q", errPortableResponseMaximumKindMissing, job.Kind)
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

func maximumAcceptedCandidateBytes(
	label string,
	candidates []string,
	accept func(string) error,
) (int, error) {
	maximum := 0
	for _, candidate := range candidates {
		if err := accept(candidate); err == nil && len(candidate) > maximum {
			maximum = len(candidate)
		}
	}
	if maximum == 0 {
		return 0, fmt.Errorf("%s has no decoder-accepted raw candidate", label)
	}
	return maximum, nil
}

func cappedResponseMaximum(maximum, ceiling int) int {
	if maximum < ceiling {
		return maximum
	}
	return ceiling
}
