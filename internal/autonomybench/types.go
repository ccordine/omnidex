package autonomybench

import (
	"context"
	"time"
)

type Status string

const (
	StatusPassed  Status = "passed"
	StatusPartial Status = "partial"
	StatusFailed  Status = "failed"
)

// RequestCase is the complete input visible to the framework under test.
type RequestCase struct {
	ID          string
	UserRequest string
	Workspace   string
}

// BuildInput deliberately contains no benchmark rubric, checks, or expected
// implementation. Adding a field here changes the benchmark trust boundary.
type BuildInput struct {
	UserRequest string
	Workspace   string
}

type BuildObservation struct {
	ModelCalls       int
	PromptBytes      int
	FilesChanged     int
	UnitsPlanned     int
	UnitsAccepted    int
	UnitsRejected    int
	CorrectionCalls  int
	VerificationRuns int
}

type Check struct {
	ID     string
	Weight int
}

type EvaluationPlan struct {
	RequestID string
	Checks    []Check
}

type EvaluationInput struct {
	Workspace string
	Checks    []Check
}

type CheckResult struct {
	ID       string
	Passed   bool
	Evidence string
}

type Result struct {
	RequestID    string
	Workspace    string
	Status       Status
	Build        BuildObservation
	BuildError   string
	EarnedWeight int
	TotalWeight  int
	Checks       []CheckResult
	Elapsed      time.Duration
}

type Builder interface {
	Build(context.Context, BuildInput) (BuildObservation, error)
}

type EvaluationLoader interface {
	Load(context.Context, string) (EvaluationPlan, error)
}

type Evaluator interface {
	Evaluate(context.Context, EvaluationInput) ([]CheckResult, error)
}

type BuilderFunc func(context.Context, BuildInput) (BuildObservation, error)

func (f BuilderFunc) Build(ctx context.Context, input BuildInput) (BuildObservation, error) {
	return f(ctx, input)
}

type EvaluationLoaderFunc func(context.Context, string) (EvaluationPlan, error)

func (f EvaluationLoaderFunc) Load(ctx context.Context, requestID string) (EvaluationPlan, error) {
	return f(ctx, requestID)
}

type EvaluatorFunc func(context.Context, EvaluationInput) ([]CheckResult, error)

func (f EvaluatorFunc) Evaluate(ctx context.Context, input EvaluationInput) ([]CheckResult, error) {
	return f(ctx, input)
}
