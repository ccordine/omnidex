package worker

import (
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

// Isolated adapter qualification uses the production body loop and provider's
// native continuation. It is not evidence of ordinary-request autonomy.
func liveSourceBodyRuntime(t *testing.T, client llm.ExactStationClient, model string, contextTokens int) typedWorkerRuntime {
	t.Helper()
	var key assemblyline.PortableJobKey
	var parent llm.PreparedGeneration
	iteration := 0
	dispatch := func(job assemblyline.PortableJob, requestedModel, prompt string, retained []int) (assemblyline.PortableResult, error) {
		if requestedModel != model || (iteration > 0 && job.Key() != key) {
			return assemblyline.PortableResult{}, fmt.Errorf("source continuation changed its immutable route or job")
		}
		key = job.Key()
		iteration++
		maxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(job, contextTokens)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		prepared, err := prepareExactStationCall(exactStationCall{
			WorkInput: string(job.Payload), WorkKind: job.Kind, Iteration: iteration, Prompt: prompt,
			ContextTokens: contextTokens, MaxOutputTokens: maxOutputTokens, RetainedContext: retained,
		}, model, nil)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		t.Logf("model=%s iteration=%d prompt_bytes=%d retained_tokens=%d prompt=%q", model, iteration, len(prompt), len(retained), prompt)
		parent, err = generatePreparedExactWithinMaximumDuration(t.Context(), client, prepared)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if err := llm.ValidateExactPreparedGenerationForRequest(prepared, parent); err != nil {
			return assemblyline.PortableResult{}, err
		}
		t.Logf("iteration=%d response=%q", iteration, parent.Content)
		return assemblyline.PortableResult{Candidate: parent.Content}, nil
	}
	return typedWorkerRuntime{
		Context: t.Context(), MaxAttempts: assemblyline.MaxSourceBodyAttempts,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			return dispatch(job, model, prompt, nil)
		},
		Correct: func(job assemblyline.PortableJob, model string, correction assemblyline.SourceBodyCorrection) (assemblyline.PortableResult, error) {
			prompt, err := correction.ModelInput()
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			retained, err := llm.DecodeExactPreparedRetainedContext(llm.ExactPreparedProtocolPlainCompletionV4, parent.ProviderResponseCapture, contextTokens)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			return dispatch(job, model, prompt, retained)
		},
		AdvanceSource: func(assemblyline.PortableJob, string, string, string) error { return nil },
		Release:       func(assemblyline.PortableJob) error { return nil },
		Finalize: func(_ assemblyline.PortableJob, _ assemblyline.PortableResult, err error) error {
			t.Logf("iteration=%d validation=%v", iteration, err)
			return nil
		},
	}
}
