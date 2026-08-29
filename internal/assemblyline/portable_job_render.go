package assemblyline

import "fmt"

type portableJobRenderer func(PortableJob) (string, bool, error)

// RenderPortableJob is the sole mapping from an immutable work envelope to
// model-visible raw context. Schedulers may choose a model or machine, but
// cannot add workspace state, instructions, or a structured response channel.
func RenderPortableJob(job PortableJob) (string, error) {
	if err := ValidatePortableJobForRenderer(job, PortableRendererV8); err != nil {
		return "", err
	}
	renderers := [...]portableJobRenderer{
		renderPortableApplicationJob,
		renderPortableRepositoryContextJob,
		renderPortableConversationRoleplayJob,
		renderPortableDatabaseWebJob,
		renderPortableCodingJob,
	}
	for _, render := range renderers {
		if prompt, handled, err := render(job); handled {
			return prompt, err
		}
	}
	return "", fmt.Errorf("portable job kind %q has no renderer", job.Kind)
}

func renderDecodedPortableInput[T any](job PortableJob, build func(T) (string, error)) (string, error) {
	var input T
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return "", err
	}
	return build(input)
}

func handledPortableRender(prompt string, err error) (string, bool, error) {
	return prompt, true, err
}
