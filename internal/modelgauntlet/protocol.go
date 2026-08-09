package modelgauntlet

import (
	"context"
	"fmt"
	"strings"
)

func runStructuredAdvisoryProtocol(
	ctx context.Context,
	config advisoryProtocolConfig,
	cases []structuredAdvisoryCase,
	generator Generator,
) (advisoryProtocolReport, error) {
	states, err := validateAdvisoryProtocol(ctx, config, cases, generator)
	if err != nil {
		return advisoryProtocolReport{}, err
	}
	report := advisoryProtocolReport{}
	for _, state := range states {
		report.Outcomes = append(report.Outcomes,
			advisoryOutcome{CaseID: state.spec.ID, Variant: VariantDirect, Error: "direct call has not run"},
			advisoryOutcome{CaseID: state.spec.ID, Variant: VariantDeliberated, Error: "deliberated calls have not run"},
		)
	}

	if err := runDirectPhase(ctx, config, states, generator, &report); err != nil {
		return report, err
	}
	if err := runBriefingPhase(ctx, config, states, generator, &report); err != nil {
		return report, err
	}
	if err := runDeliberationPhase(ctx, config, states, generator, &report); err != nil {
		return report, err
	}
	if err := runSynthesisPhase(ctx, config, states, generator, &report); err != nil {
		return report, err
	}
	return report, nil
}

func runDirectPhase(
	ctx context.Context,
	config advisoryProtocolConfig,
	states []*advisoryCaseState,
	generator Generator,
	report *advisoryProtocolReport,
) error {
	for _, state := range states {
		request, err := structuredAdvisoryRequest(
			config, state.spec.ID, VariantDirect, StageDirect,
			state.authoritativePrompt, state.responseSchema, maxStructuredTokens,
		)
		if err != nil {
			return fmt.Errorf("render direct response contract for case %q: %w", state.spec.ID, err)
		}
		response, evidence, callErr := callWithEvidence(ctx, generator, request)
		report.Calls = append(report.Calls, evidence)
		setAdvisoryOutcome(report, state.spec, VariantDirect, response, callErr)
	}
	return nil
}

func runBriefingPhase(
	ctx context.Context,
	config advisoryProtocolConfig,
	states []*advisoryCaseState,
	generator Generator,
	report *advisoryProtocolReport,
) error {
	for _, state := range states {
		prompt, err := state.spec.Station.buildBriefingPrompt(state.authoritativePrompt)
		if err != nil {
			return fmt.Errorf("render briefing prompt for case %q: %w", state.spec.ID, err)
		}
		request, err := structuredAdvisoryRequest(
			config, state.spec.ID, VariantDeliberated, StageBriefing,
			prompt, state.spec.Station.briefingResponseSchema(), maxLensTokens,
		)
		if err != nil {
			return fmt.Errorf("render briefing response contract for case %q: %w", state.spec.ID, err)
		}
		response, evidence, callErr := callWithEvidence(ctx, generator, request)
		report.Calls = append(report.Calls, evidence)
		if callErr != nil {
			invalidateAdvisoryOutcome(report, state.spec.ID, VariantDeliberated, "briefing failed: "+callErr.Error())
			continue
		}
		briefing, err := state.spec.Station.decodeBriefing(response.Content)
		if err != nil {
			invalidateAdvisoryOutcome(report, state.spec.ID, VariantDeliberated, err.Error())
			continue
		}
		state.briefing = briefing
	}
	return nil
}

func runDeliberationPhase(
	ctx context.Context,
	config advisoryProtocolConfig,
	states []*advisoryCaseState,
	generator Generator,
	report *advisoryProtocolReport,
) error {
	for _, state := range states {
		if state.briefing == nil {
			continue
		}
		prompt, err := state.spec.Station.buildDeliberationPrompt(state.authoritativePrompt, state.briefing)
		if err != nil {
			return fmt.Errorf("render deliberation prompt for case %q: %w", state.spec.ID, err)
		}
		request := GenerateRequest{
			CaseID: state.spec.ID, Variant: VariantDeliberated, Stage: StageDeliberation,
			Model: config.ReasoningModel, SystemPrompt: prompt,
			UserPrompt: "Produce the bounded advisory memo now.", Think: true,
			MaxOutputTokens: maxDeliberationTokens, ContextTokens: config.ContextTokens, KeepAlive: config.KeepAlive,
		}
		response, evidence, callErr := callWithEvidence(ctx, generator, request)
		report.Calls = append(report.Calls, evidence)
		if callErr != nil {
			invalidateAdvisoryOutcome(report, state.spec.ID, VariantDeliberated, "deliberation failed: "+callErr.Error())
			continue
		}
		if strings.TrimSpace(response.Content) == "" {
			invalidateAdvisoryOutcome(report, state.spec.ID, VariantDeliberated, "deliberation returned no final memo content")
			continue
		}
		state.deliberation = rawDeliberation{Content: response.Content}
		if err := validateDeliberationSize(state.deliberation); err != nil {
			invalidateAdvisoryOutcome(report, state.spec.ID, VariantDeliberated, err.Error())
			continue
		}
		state.deliberationOkay = true
	}
	return nil
}

func runSynthesisPhase(
	ctx context.Context,
	config advisoryProtocolConfig,
	states []*advisoryCaseState,
	generator Generator,
	report *advisoryProtocolReport,
) error {
	for _, state := range states {
		if !state.deliberationOkay {
			continue
		}
		prompt, err := buildAdvisorySynthesisPrompt(state)
		if err != nil {
			return fmt.Errorf("render synthesis prompt for case %q: %w", state.spec.ID, err)
		}
		request, err := structuredAdvisoryRequest(
			config, state.spec.ID, VariantDeliberated, StageSynthesis,
			prompt, state.responseSchema, maxStructuredTokens,
		)
		if err != nil {
			return fmt.Errorf("render synthesis response contract for case %q: %w", state.spec.ID, err)
		}
		response, evidence, callErr := callWithEvidence(ctx, generator, request)
		report.Calls = append(report.Calls, evidence)
		setAdvisoryOutcome(report, state.spec, VariantDeliberated, response, callErr)
	}
	return nil
}

func buildAdvisorySynthesisPrompt(state *advisoryCaseState) (string, error) {
	if renderer, ok := state.spec.Station.(advisorySynthesisRenderer); ok {
		return renderer.buildSynthesisPrompt(state.authoritativePrompt, state.deliberation)
	}
	return buildBoundedSynthesisPrompt(
		state.spec.Station.synthesisInstruction(), state.authoritativePrompt, state.deliberation,
	)
}
