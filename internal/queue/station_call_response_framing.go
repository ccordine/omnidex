package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

// ExpectedStationCallStopSequence resolves one provider-actionable terminator
// from code-owned work grammar and the discovered provider transport.
func ExpectedStationCallStopSequence(
	gap StationGapOpening,
	expected llm.ProviderIdentityExpectation,
) (string, error) {
	kind := assemblyline.WorkKind(gap.WorkKind)
	transport, err := assemblyline.PortableResponseTransportForWorkKind(kind)
	if err != nil {
		return "", err
	}
	framing, err := stationGapResponseFraming(gap, kind)
	if err != nil {
		return "", err
	}
	settings, err := llm.ResolveExactPreparedTransport(expected)
	if err != nil {
		return "", err
	}
	switch {
	case transport == assemblyline.PortableResponseTransportSemanticRaw &&
		framing == assemblyline.PortableResponseFramingSingleLine:
		return llm.ExactPreparedLineStopV1, nil
	case framing == assemblyline.PortableResponseFramingNaturalMultiline &&
		settings.NativeTemplate:
		return "", nil
	case (transport == assemblyline.PortableResponseTransportSemanticRaw ||
		transport == assemblyline.PortableResponseTransportStructuralRaw ||
		transport == assemblyline.PortableResponseTransportFragmentRaw) &&
		framing == assemblyline.PortableResponseFramingNaturalMultiline:
		return llm.ExactPreparedRawChatEndV1, nil
	default:
		return "", fmt.Errorf(
			"station gap work kind %q has mismatched response transport %q and framing %q",
			kind, transport, framing,
		)
	}
}

func stationGapResponseFraming(
	_ StationGapOpening,
	kind assemblyline.WorkKind,
) (assemblyline.PortableResponseFraming, error) {
	return assemblyline.PortableResponseFramingForWorkKind(kind)
}
