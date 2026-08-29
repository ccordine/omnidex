package assemblyline

import (
	"encoding/json"
	"fmt"
)

func validatePortableJobPayloadForRenderer(
	kind WorkKind,
	payload json.RawMessage,
	renderer string,
) error {
	if renderer != PortableRendererV1 {
		return fmt.Errorf("portable renderer %q is not registered", renderer)
	}
	return validatePortableJobPayload(kind, payload)
}
