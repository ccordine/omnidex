package assemblyline

import (
	"encoding/json"
	"fmt"
)

func validatePortableJobPayload(kind WorkKind, payload json.RawMessage) error {
	switch kind {
	case WorkApplicationClassify:
		return decodeAndValidatePortablePayload[ApplicationClassificationInput](payload, ApplicationClassificationInput.validate)
	case WorkApplicationIdentity:
		return decodeAndValidatePortablePayload[ApplicationIdentityInput](payload, ApplicationIdentityInput.validate)
	case WorkRequirementPartition:
		return decodeAndValidatePortablePayload[RequirementPartitionInput](payload, RequirementPartitionInput.validate)
	case WorkArtifactHandling:
		return decodeAndValidatePortablePayload[ArtifactHandlingInput](payload, ArtifactHandlingInput.validate)
	case WorkCapabilityRelation:
		return decodeAndValidatePortablePayload[CapabilityRelationInput](payload, CapabilityRelationInput.validate)
	case WorkSkillProcedure:
		return decodeAndValidatePortablePayload[SkillProcedureInput](payload, SkillProcedureInput.validate)
	case WorkSkillSelection:
		return decodeAndValidatePortablePayload[SkillSelectionInput](payload, SkillSelectionInput.validate)
	case WorkFragmentGeneration:
		return decodeAndValidatePortablePayload[FragmentGenerationInput](payload, FragmentGenerationInput.validate)
	case WorkFragmentCorrection:
		return decodeAndValidatePortablePayload[FragmentCorrectionInput](payload, FragmentCorrectionInput.validate)
	case WorkResponseCorrection:
		return decodeAndValidatePortablePayload[ResponseCorrectionInput](payload, ResponseCorrectionInput.validate)
	default:
		return fmt.Errorf("portable job kind %q has no payload validator", kind)
	}
}

func decodeAndValidatePortablePayload[T any](payload json.RawMessage, validate func(T) error) error {
	var input T
	if err := decodePortablePayload(payload, &input); err != nil {
		return err
	}
	return validate(input)
}
