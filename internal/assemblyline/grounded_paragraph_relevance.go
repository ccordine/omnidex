package assemblyline

import "fmt"

func NewGroundedParagraphRelevanceJob(input GroundedParagraphRelevanceInput) (PortableJob, error) {
	return newPortableJob(WorkGroundedParagraphRelevance, input)
}

func BuildGroundedParagraphRelevancePrompt(input GroundedParagraphRelevanceInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	context, err := renderGroundedAnswerModelContext(input.ExactQuestion, input.Context, input.ParagraphText, nil)
	if err != nil {
		return "", err
	}
	choices, err := groundedParagraphRelevanceChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Does this exact candidate paragraph address the meaning of the question?", []string{context}, choices,
	)
}

func DecodeGroundedParagraphRelevance(input GroundedParagraphRelevanceInput, raw string) (GroundedParagraphRelevance, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := groundedParagraphRelevanceChoices()
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	return GroundedParagraphRelevance(value), err
}

func groundedParagraphRelevanceChoices() ([]OpaqueModelChoice, error) {
	relevant, err := NewOpaqueModelChoice("The paragraph addresses the question.", string(GroundedParagraphRelevant))
	if err != nil {
		return nil, err
	}
	unrelated, err := NewOpaqueModelChoice("The paragraph does not address the question.", string(GroundedParagraphNotRelevant))
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{relevant, unrelated}, nil
}

func (relation GroundedParagraphRelevance) ValidateFor(input GroundedParagraphRelevanceInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if relation != GroundedParagraphRelevant && relation != GroundedParagraphNotRelevant {
		return fmt.Errorf("grounded paragraph relevance %q is not registered", relation)
	}
	return nil
}
