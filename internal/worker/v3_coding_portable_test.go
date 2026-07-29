package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func testPortableExecutor(
	generate func(scope, model, prompt string, responseSchema map[string]any) (string, error),
) func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
	return func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
		prompt, schema, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		raw, err := generate(portableModelScope(schema), model, prompt, schema)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		return assemblyline.PortableResult{JobID: job.ID, Candidate: raw}, nil
	}
}
