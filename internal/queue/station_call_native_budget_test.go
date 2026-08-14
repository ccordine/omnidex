package queue

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestStationCallOpeningPersistsDeclaredTokenCeilingWithoutGuessingFromBytes(t *testing.T) {
	t.Parallel()

	authority := model.StepAttemptAuthority{JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a"}
	gap := stationCallTestGap(t, authority)
	gap.ContextTokens = 8192
	gap.MaxOutputTokens = 2048
	const measuredModelInputBytes = 6485
	gap.Prompt = strings.Repeat(
		"x",
		measuredModelInputBytes-len(llm.ExactPreparedPromptJoiner)-len(llm.MinimalGeneratePrompt),
	)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap,
		Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatalf("station opening guessed %d measured bytes were tokens: %v", measuredModelInputBytes, err)
	}
	if opening.ModelInputBytes != measuredModelInputBytes {
		t.Fatalf("model input bytes=%d want %d", opening.ModelInputBytes, measuredModelInputBytes)
	}
	if opening.MaxInputTokens != 6144 ||
		opening.ModelInputTokenCeiling != opening.MaxInputTokens {
		t.Fatalf(
			"persisted token authority=%d max_input=%d, want the declared 6144-token ceiling",
			opening.ModelInputTokenCeiling, opening.MaxInputTokens,
		)
	}
	if opening.ModelInputTokenCeiling == opening.ModelInputBytes+2 {
		t.Fatal("station opening persisted the obsolete byte-plus-two token guess")
	}
}

func TestStationCallNativeUsageRejectsProviderCountsBeyondOpenedAuthority(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a"}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap,
		Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.ID = 23

	for name, counts := range map[string]struct{ prompt, output int }{
		"input ceiling":  {prompt: opening.MaxInputTokens + 1, output: 7},
		"output ceiling": {prompt: 41, output: opening.MaxOutputTokens + 1},
	} {
		name, counts := name, counts
		t.Run(name, func(t *testing.T) {
			result := stationCallSuccessWithUsage(t, prepared, opening, counts.prompt, counts.output)
			if err := ValidateStationCallNativeUsage(opening, result); err == nil ||
				!strings.Contains(err.Error(), "exact provider context exceeded") {
				t.Fatalf("out-of-authority native usage error=%v", err)
			}
		})
	}
}

func stationCallSuccessWithUsage(
	t *testing.T,
	prepared llm.PreparedModel,
	opening StationCallOpening,
	promptTokens int,
	outputTokens int,
) llm.PreparedGeneration {
	t.Helper()
	result := stationCallSuccess(t, prepared, opening)
	body := strings.Replace(
		string(result.ProviderResponseCapture),
		`"prompt_eval_count":41`, fmt.Sprintf(`"prompt_eval_count":%d`, promptTokens), 1,
	)
	body = strings.Replace(
		body, `"eval_count":7`, fmt.Sprintf(`"eval_count":%d`, outputTokens), 1,
	)
	decoded, err := llm.DecodeExactPreparedResponseForProtocol(prepared.Protocol, 200, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	result.ProviderResponseCapture = []byte(body)
	result.ProviderResponseBytes = int64(len(body))
	result.ProviderResponseCapturedBytes = len(body)
	result.ProviderResponseSHA256 = stationGapSHA256(body)
	result.ProviderResponseCaptureSHA256 = stationGapSHA256(body)
	result.UsagePresent = decoded.UsagePresent
	result.Usage = decoded.Usage
	return result
}
