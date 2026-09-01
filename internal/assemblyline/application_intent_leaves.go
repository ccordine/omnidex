package assemblyline

import "strings"

const WorkApplicationProductContext WorkKind = "application_product_context"

type ApplicationProductContextInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
}

func NewApplicationProductContextJob(
	input ApplicationProductContextInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkApplicationProductContext, input,
	)
}

func (input ApplicationProductContextInput) validate() error {
	return (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate()
}

func BuildApplicationProductContextPrompt(
	input ApplicationProductContextInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := renderApplicationContextModelProjection(input.UserRequest, input.Context)
	return strings.Join([]string{
		"What concise product or domain identity is explicitly established by this software request and its established facts?",
		"Product context contains only the product identity, subject or domain, intended audience, and stated setting or purpose. Exclude requested qualities, capabilities, behaviors, user-visible elements, state or persistence, artifact or format constraints, accessibility or responsiveness, tests, build or deployment constraints, and implementation detail.",
		projection,
	}, "\n\n"), nil
}

func DecodeApplicationProductContextLeaf(
	input ApplicationProductContextInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application product context", raw, maxApplicationProductBytes, true,
	)
	if err != nil {
		return "", err
	}
	if err := validateApplicationIntentText(
		"product context", leaf, maxApplicationProductBytes,
	); err != nil {
		return "", err
	}
	return leaf, nil
}
