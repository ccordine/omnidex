package modelgauntlet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

type advisoryProtocolConfig struct {
	StableModel    string
	ReasoningModel string
	ContextTokens  int
	KeepAlive      string
}

type advisoryBriefing interface {
	advisoryBriefing()
}

type advisoryStation interface {
	workKind() assemblyline.WorkKind
	buildBriefingPrompt(authoritativePrompt string) (string, error)
	briefingResponseSchema() map[string]any
	decodeBriefing(raw string) (advisoryBriefing, error)
	buildDeliberationPrompt(authoritativePrompt string, briefing advisoryBriefing) (string, error)
	synthesisInstruction() string
	validateCandidate(raw string) error
}

type advisorySynthesisRenderer interface {
	buildSynthesisPrompt(authoritativePrompt string, memo rawDeliberation) (string, error)
}

type structuredAdvisoryCase struct {
	ID      string
	Job     assemblyline.PortableJob
	Station advisoryStation
}

type advisoryOutcome struct {
	CaseID  string
	Variant Variant
	Valid   bool
	Content string
	Error   string
}

type advisoryProtocolReport struct {
	Calls    []CallEvidence
	Outcomes []advisoryOutcome
}

type advisoryCaseState struct {
	spec                structuredAdvisoryCase
	authoritativePrompt string
	responseSchema      map[string]any
	briefing            advisoryBriefing
	deliberation        rawDeliberation
	deliberationOkay    bool
}

func validateAdvisoryProtocol(
	ctx context.Context,
	config advisoryProtocolConfig,
	cases []structuredAdvisoryCase,
	generator Generator,
) ([]*advisoryCaseState, error) {
	if ctx == nil {
		return nil, fmt.Errorf("structured advisory protocol requires a context")
	}
	if generator == nil {
		return nil, fmt.Errorf("structured advisory protocol requires a generator")
	}
	if err := validateAdvisoryConfig(config); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("structured advisory protocol requires at least one case")
	}
	seen := make(map[string]struct{}, len(cases))
	states := make([]*advisoryCaseState, 0, len(cases))
	for _, spec := range cases {
		if strings.TrimSpace(spec.ID) == "" || spec.ID != strings.TrimSpace(spec.ID) {
			return nil, fmt.Errorf("structured advisory case requires one trimmed ID")
		}
		if _, exists := seen[spec.ID]; exists {
			return nil, fmt.Errorf("structured advisory case ID %q is duplicated", spec.ID)
		}
		seen[spec.ID] = struct{}{}
		if err := spec.Job.Validate(); err != nil {
			return nil, fmt.Errorf("structured advisory case %q has invalid job: %w", spec.ID, err)
		}
		if err := validateAdvisoryWorkKind(spec.Job.Kind); err != nil {
			return nil, fmt.Errorf("structured advisory case %q: %w", spec.ID, err)
		}
		if spec.Station == nil {
			return nil, fmt.Errorf("structured advisory case %q requires a registered station", spec.ID)
		}
		if spec.Station.workKind() != spec.Job.Kind {
			return nil, fmt.Errorf("structured advisory case %q station kind %q does not match job kind %q", spec.ID, spec.Station.workKind(), spec.Job.Kind)
		}
		prompt, schema, err := assemblyline.RenderPortableJob(spec.Job)
		if err != nil {
			return nil, fmt.Errorf("render authoritative job for case %q: %w", spec.ID, err)
		}
		if len(schema) == 0 {
			return nil, fmt.Errorf("structured advisory case %q authoritative job has no response schema", spec.ID)
		}
		states = append(states, &advisoryCaseState{
			spec: spec, authoritativePrompt: prompt, responseSchema: schema,
		})
	}
	return states, nil
}

func validateAdvisoryWorkKind(kind assemblyline.WorkKind) error {
	switch kind {
	case assemblyline.WorkApplicationClassify, assemblyline.WorkApplicationIdentity,
		assemblyline.WorkRequirementPartition, assemblyline.WorkArtifactHandling,
		assemblyline.WorkCapabilityRelation, assemblyline.WorkSkillSelection,
		assemblyline.WorkSkillProcedure, assemblyline.WorkRepositoryRetrieval:
		return nil
	case assemblyline.WorkFragmentGeneration, assemblyline.WorkFragmentCorrection,
		assemblyline.WorkResponseCorrection:
		return fmt.Errorf("structured advisory protocol kind %q is unsupported: coding and correction jobs must retain their authoritative portable envelope", kind)
	default:
		return fmt.Errorf("structured advisory protocol kind %q is unsupported", kind)
	}
}

func validateAdvisoryConfig(config advisoryProtocolConfig) error {
	if strings.TrimSpace(config.StableModel) == "" || config.StableModel != strings.TrimSpace(config.StableModel) {
		return fmt.Errorf("structured advisory protocol requires one trimmed stable model")
	}
	if strings.TrimSpace(config.ReasoningModel) == "" || config.ReasoningModel != strings.TrimSpace(config.ReasoningModel) {
		return fmt.Errorf("structured advisory protocol requires one trimmed reasoning model")
	}
	if config.ContextTokens <= maxDeliberationTokens {
		return fmt.Errorf("context tokens must exceed the %d-token deliberation output budget", maxDeliberationTokens)
	}
	if err := llm.ValidateInferenceContextTokens(config.ContextTokens); err != nil {
		return err
	}
	if strings.TrimSpace(config.KeepAlive) == "" || config.KeepAlive != strings.TrimSpace(config.KeepAlive) {
		return fmt.Errorf("structured advisory protocol requires one trimmed keep-alive value")
	}
	retention, err := time.ParseDuration(config.KeepAlive)
	if err != nil || retention <= 0 {
		return fmt.Errorf("structured advisory protocol keep alive must be a positive duration, received %q", config.KeepAlive)
	}
	return nil
}
