package omni

import (
	"context"
)

type fakeCommandDecisionClient struct {
	responses []string
	errors    []error
	calls     int
	prompts   []string
	requests  []OllamaChatRequest
}

func (f *fakeCommandDecisionClient) ChatRaw(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error) {
	f.calls++
	f.requests = append(f.requests, req)
	if len(req.Messages) > 0 {
		f.prompts = append(f.prompts, req.Messages[len(req.Messages)-1].Content)
	}
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return OllamaChatResponse{}, err
		}
	}
	if len(f.responses) == 0 {
		return OllamaChatResponse{Content: `{"command":"","done":true,"answer":"done"}`}, nil
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return OllamaChatResponse{Content: response}, nil
}

type fakeStructuredResponseEvaluator struct {
	evaluations []StructuredLLMEvaluation
	errors      []error
	inputs      []StructuredLLMEvaluationInput
}

func (f *fakeStructuredResponseEvaluator) EvaluateStructuredLLMResponse(ctx context.Context, input StructuredLLMEvaluationInput) (StructuredLLMEvaluation, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return StructuredLLMEvaluation{}, err
		}
	}
	if len(f.evaluations) == 0 {
		return StructuredLLMEvaluation{Confidence: 100, Feedback: ""}, nil
	}
	evaluation := f.evaluations[0]
	f.evaluations = f.evaluations[1:]
	return evaluation, nil
}

type fakeShellCommandSpecialist struct {
	proposals []ShellCommandProposal
	errors    []error
	inputs    []ShellCommandSpecialistInput
}

type fakeCodeContentSpecialist struct {
	proposals []CodeContentProposal
	errors    []error
	inputs    []CodeContentSpecialistInput
}

type fakeCursorArchitectAgent struct {
	results []CursorArchitectAgentResult
	errors  []error
	inputs  []CursorArchitectAgentInput
	run     func(CursorArchitectAgentInput) error
}

type fakeStreamingArchitectAgent struct {
	events []AgentEvent
	inputs []CursorArchitectAgentInput
}

type fakeExternalAgentSession struct {
	events       []AgentEvent
	startedJobs  []ExternalAgentJob
	cancelCount  int
	cleanupCount int
}

type fakeUserAssistanceSpecialist struct {
	questions []UserAssistanceQuestion
	errors    []error
	inputs    []UserAssistanceInput
}

func (f *fakeUserAssistanceSpecialist) BuildUserAssistanceQuestion(ctx context.Context, input UserAssistanceInput) (UserAssistanceQuestion, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return UserAssistanceQuestion{}, err
		}
	}
	if len(f.questions) == 0 {
		return UserAssistanceQuestion{}, nil
	}
	question := f.questions[0]
	f.questions = f.questions[1:]
	return question, nil
}

func (f *fakeCodeContentSpecialist) GenerateCodeContent(ctx context.Context, input CodeContentSpecialistInput) (CodeContentProposal, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return CodeContentProposal{}, err
		}
	}
	if len(f.proposals) == 0 {
		return CodeContentProposal{Content: "export default function App() { return null; }\n", Rationale: "default fake content"}, nil
	}
	proposal := f.proposals[0]
	f.proposals = f.proposals[1:]
	return proposal, nil
}

func (f *fakeCursorArchitectAgent) RunArchitectTask(ctx context.Context, input CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	f.inputs = append(f.inputs, input)
	if f.run != nil {
		if err := f.run(input); err != nil {
			return CursorArchitectAgentResult{}, err
		}
	}
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return CursorArchitectAgentResult{}, err
		}
	}
	if len(f.results) == 0 {
		return CursorArchitectAgentResult{Summary: "cursor completed"}, nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result, nil
}

func (f *fakeStreamingArchitectAgent) RunArchitectTask(ctx context.Context, input CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	return CursorArchitectAgentResult{Summary: "fallback should not run"}, nil
}

func (f *fakeStreamingArchitectAgent) NewExternalAgentSession(input CursorArchitectAgentInput) (ExternalAgentSession, error) {
	f.inputs = append(f.inputs, input)
	return &fakeExternalAgentSession{events: f.events}, nil
}

func (s *fakeExternalAgentSession) Start(ctx context.Context, job ExternalAgentJob) (<-chan AgentEvent, error) {
	s.startedJobs = append(s.startedJobs, job)
	ch := make(chan AgentEvent, len(s.events))
	for _, event := range s.events {
		if event.SessionID == "" {
			event.SessionID = job.SessionID
		}
		if event.Agent == "" {
			event.Agent = job.Agent
		}
		ch <- event
	}
	close(ch)
	return ch, nil
}

func (s *fakeExternalAgentSession) Interrupt(ctx context.Context, correction HumanCorrection) error {
	return nil
}
func (s *fakeExternalAgentSession) Cancel(ctx context.Context, reason string) error {
	s.cancelCount++
	return nil
}
func (s *fakeExternalAgentSession) Pause(ctx context.Context) error  { return nil }
func (s *fakeExternalAgentSession) Resume(ctx context.Context) error { return nil }
func (s *fakeExternalAgentSession) Cleanup(ctx context.Context) error {
	s.cleanupCount++
	return nil
}

type fakePromptInterpreter struct {
	interpretations []PromptInterpretation
	errors          []error
	inputs          []PromptInterpretationInput
}

func requiredCommandObjectiveForTest(id, description, command string) StructuredObjective {
	return StructuredObjective{
		ID:               id,
		Description:      description,
		Status:           "pending",
		Kind:             string(WorkItemKindVerify),
		RequiredEvidence: []string{"command_passed:" + command},
		Source:           structuredObjectiveSourceUserExplicit,
		Required:         true,
	}
}

func (f *fakePromptInterpreter) InterpretPrompt(ctx context.Context, input PromptInterpretationInput) (PromptInterpretation, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return PromptInterpretation{}, err
		}
	}
	if len(f.interpretations) == 0 {
		return PromptInterpretation{}, nil
	}
	interpretation := f.interpretations[0]
	f.interpretations = f.interpretations[1:]
	return interpretation, nil
}

type fakeContextSummarizer struct {
	contexts []MinimalContext
	errors   []error
	inputs   []MinimalContextInput
}

type fakeCompletionChecker struct {
	checks []CompletionCheck
	errors []error
	inputs []CompletionCheckInput
}

func (f *fakeCompletionChecker) CheckCompletion(ctx context.Context, input CompletionCheckInput) (CompletionCheck, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return CompletionCheck{}, err
		}
	}
	if len(f.checks) == 0 {
		return CompletionCheck{}, nil
	}
	check := f.checks[0]
	f.checks = f.checks[1:]
	return check, nil
}

func (f *fakeContextSummarizer) SummarizeContext(ctx context.Context, input MinimalContextInput) (MinimalContext, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return MinimalContext{}, err
		}
	}
	if len(f.contexts) == 0 {
		return MinimalContext{}, nil
	}
	context := f.contexts[0]
	f.contexts = f.contexts[1:]
	return context, nil
}

func (f *fakeShellCommandSpecialist) ProposeShellCommand(ctx context.Context, input ShellCommandSpecialistInput) (ShellCommandProposal, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return ShellCommandProposal{}, err
		}
	}
	if len(f.proposals) == 0 {
		return ShellCommandProposal{Command: "printf 'default shell evidence\n'", Rationale: "default"}, nil
	}
	proposal := f.proposals[0]
	f.proposals = f.proposals[1:]
	return proposal, nil
}
