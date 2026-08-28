package assemblyline

import "fmt"

func DecodeApplicationClassification(
	input ApplicationClassificationInput,
	raw string,
) (ApplicationClassification, error) {
	if err := input.validate(); err != nil {
		return ApplicationClassification{}, err
	}
	leaf, err := decodeRawSemanticLeaf("application surface", raw, 64, false)
	if err != nil {
		return ApplicationClassification{}, err
	}
	classification := ApplicationClassification{
		Schema:  ApplicationClassificationSchemaV1,
		Surface: ApplicationSurface(leaf),
	}
	if err := classification.Validate(); err != nil {
		return ApplicationClassification{}, err
	}
	return classification, nil
}

func (decision ArtifactHandlingDecision) Validate(token string) error {
	if decision.Schema != ArtifactHandlingSchemaV1 {
		return fmt.Errorf("artifact handling schema must be %q", ArtifactHandlingSchemaV1)
	}
	if decision.Token != token {
		return fmt.Errorf("artifact handling token %q does not match focused token %q", decision.Token, token)
	}
	switch decision.Handling {
	case ArtifactPreserveUnchanged, ArtifactMustExist, ArtifactMustBeAbsent,
		ArtifactPossibleAbsenceCandidate, ArtifactMentionedOnly:
		return nil
	default:
		return fmt.Errorf("artifact handling %q is unsupported", decision.Handling)
	}
}

func DecodeArtifactHandlingDecision(
	input ArtifactHandlingInput,
	raw string,
) (ArtifactHandlingDecision, error) {
	if err := input.validate(); err != nil {
		return ArtifactHandlingDecision{}, err
	}
	leaf, err := decodeRawSemanticLeaf("artifact handling", raw, 64, false)
	if err != nil {
		return ArtifactHandlingDecision{}, err
	}
	decision := ArtifactHandlingDecision{
		Schema:   ArtifactHandlingSchemaV1,
		Token:    input.Token,
		Handling: ArtifactHandling(leaf),
	}
	if err := decision.Validate(input.Token); err != nil {
		return ArtifactHandlingDecision{}, err
	}
	return decision, nil
}
