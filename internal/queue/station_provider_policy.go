package queue

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// PortableJobUsesRoleplayRawCompletion is the single code-owned classifier
// shared by durable gap validation and provider selection. It inspects typed
// work, never user wording.
func PortableJobUsesRoleplayRawCompletion(job assemblyline.PortableJob) (bool, error) {
	if err := job.Validate(); err != nil {
		return false, err
	}
	switch job.Kind {
	case assemblyline.WorkRoleplayGroundedResponse:
		return true, nil
	case assemblyline.WorkConversationResponse:
		var input assemblyline.ConversationResponseInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return false, fmt.Errorf("decode conversation provider policy: %w", err)
		}
		return input.RoleplayIdentity != nil, nil
	case assemblyline.WorkResponseCorrection:
		var input assemblyline.ResponseCorrectionInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return false, fmt.Errorf("decode response-correction provider policy: %w", err)
		}
		return PortableJobUsesRoleplayRawCompletion(input.Original)
	default:
		return false, nil
	}
}
