package assemblyline

import "fmt"

// PortableResponseTransport is code-owned authority for the exact kind of
// bytes one portable station may return. Model wording and response schemas
// never select transport.
type PortableResponseTransport string

const (
	PortableResponseTransportSemanticRaw PortableResponseTransport = "semantic_raw"
	PortableResponseTransportFragmentRaw PortableResponseTransport = "fragment_raw"

	PortableSemanticWorkerScope = "portable_semantic_worker"
	PortableFragmentWorkerScope = "portable_fragment_worker"
)

// PortableResponseTransportForWorkKind returns the one registered response
// transport for a production portable work kind.
func PortableResponseTransportForWorkKind(
	kind WorkKind,
) (PortableResponseTransport, error) {
	if !validWorkKind(kind) {
		return "", fmt.Errorf("portable work kind %q has no registered response transport", kind)
	}
	switch kind {
	case WorkFragmentGeneration:
		return PortableResponseTransportFragmentRaw, nil
	default:
		return PortableResponseTransportSemanticRaw, nil
	}
}

func (transport PortableResponseTransport) WorkerScope() (string, error) {
	switch transport {
	case PortableResponseTransportSemanticRaw:
		return PortableSemanticWorkerScope, nil
	case PortableResponseTransportFragmentRaw:
		return PortableFragmentWorkerScope, nil
	default:
		return "", fmt.Errorf("portable response transport %q is not registered", transport)
	}
}

func PortableWorkerScopeForWorkKind(kind WorkKind) (string, error) {
	transport, err := PortableResponseTransportForWorkKind(kind)
	if err != nil {
		return "", err
	}
	return transport.WorkerScope()
}
