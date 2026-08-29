package queue

import (
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

// ExpectedPortableStationMaxOutputTokens derives the sole persisted natural
// output ceiling from the exact decoder bound and its code-owned stop grammar.
// The attested byte-level raw profile cannot require more generated tokens
// than accepted response bytes plus the stop bytes. Native profiles retain
// this value as acceptance authority even when their attested template owns
// termination and therefore omits num_predict.
func ExpectedPortableStationMaxOutputTokens(
	job assemblyline.PortableJob,
	contextTokens int,
) (int, error) {
	if err := llm.ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return 0, err
	}
	responseBytes, err := assemblyline.PortableResponseMaximumBytesForJob(job)
	if err != nil {
		return 0, fmt.Errorf("derive portable response byte ceiling: %w", err)
	}
	framing, err := assemblyline.PortableResponseFramingForJob(job)
	if err != nil {
		return 0, fmt.Errorf("derive portable response stop reserve: %w", err)
	}
	reserve := 0
	switch framing {
	case assemblyline.PortableResponseFramingSingleLine:
		reserve = len(llm.ExactPreparedLineStopV1)
	case assemblyline.PortableResponseFramingNaturalMultiline:
		reserve = len(llm.ExactPreparedRawChatEndV1)
	default:
		return 0, fmt.Errorf("portable response framing %q has no output reserve", framing)
	}
	if responseBytes < 1 || responseBytes > math.MaxInt-reserve {
		return 0, fmt.Errorf("portable response byte ceiling cannot form a positive output authority")
	}
	ceiling := responseBytes + reserve
	if ceiling > contextTokens {
		ceiling = contextTokens
	}
	return ceiling, nil
}
