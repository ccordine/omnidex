package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
)

type Spec struct {
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	InputSchema     Schema    `json:"input_schema"`
	OutputSchema    Schema    `json:"output_schema"`
	RequireEvidence bool      `json:"require_evidence,omitempty"`
	Aliases         []string  `json:"aliases,omitempty"`
	Examples        []Example `json:"examples,omitempty"`
}

type Example struct {
	When  string         `json:"when,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type Call struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input,omitempty"`
}

type Result struct {
	Tool     string         `json:"tool,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Output   map[string]any `json:"output,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
	Evidence []evidence.Record
	Accepted bool `json:"accepted"`
}

type ExecuteOptions struct {
	Allowed       []string
	Forbidden     []string
	RequireListed bool
}

type Handler func(context.Context, Call) (Result, error)

type Registry struct {
	tools map[string]registeredTool
}

type registeredTool struct {
	spec    Spec
	handler Handler
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]registeredTool{}}
}

func (r *Registry) Register(spec Spec, handler Handler) error {
	if r == nil {
		return fmt.Errorf("tool registry is nil")
	}
	if handler == nil {
		return fmt.Errorf("tool %q handler is nil", spec.Name)
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	name := normalizeName(spec.Name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", spec.Name)
	}
	spec.Name = name
	r.tools[name] = registeredTool{spec: spec, handler: handler}
	return nil
}

func (r *Registry) Execute(ctx context.Context, call Call, opts ExecuteOptions) (Result, error) {
	if r == nil {
		return Result{}, fmt.Errorf("tool registry is nil")
	}
	name := normalizeName(call.Name)
	registered, ok := r.tools[name]
	if !ok {
		return Result{}, fmt.Errorf("tool %q is not registered", call.Name)
	}
	if err := ensureAllowed(registered.spec, opts); err != nil {
		return Result{}, err
	}
	if call.Input == nil {
		call.Input = map[string]any{}
	}
	if err := registered.spec.InputSchema.ValidateValue(call.Input); err != nil {
		return Result{}, fmt.Errorf("tool %s input rejected: %w", registered.spec.Name, err)
	}
	result, err := registered.handler(ctx, Call{Name: registered.spec.Name, Input: call.Input})
	if err != nil {
		return Result{}, err
	}
	if result.Output == nil {
		result.Output = map[string]any{}
	}
	if err := registered.spec.OutputSchema.ValidateValue(result.Output); err != nil {
		return Result{}, fmt.Errorf("tool %s output rejected: %w", registered.spec.Name, err)
	}
	if registered.spec.RequireEvidence && len(result.Evidence) == 0 {
		return Result{}, fmt.Errorf("tool %s result rejected: missing evidence", registered.spec.Name)
	}
	result.Tool = registered.spec.Name
	result.Accepted = true
	if strings.TrimSpace(result.Summary) == "" {
		result.Summary = registered.spec.Name + " completed"
	}
	return result, nil
}

func (r *Registry) Spec(name string) (Spec, bool) {
	if r == nil {
		return Spec{}, false
	}
	registered, ok := r.tools[normalizeName(name)]
	if !ok {
		return Spec{}, false
	}
	return registered.spec, true
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) SpecsFor(opts ExecuteOptions) []Spec {
	if r == nil {
		return nil
	}
	out := make([]Spec, 0, len(r.tools))
	for _, name := range r.Names() {
		spec, ok := r.Spec(name)
		if !ok {
			continue
		}
		if err := ensureAllowed(spec, opts); err != nil {
			continue
		}
		out = append(out, spec)
	}
	return out
}

func (s Spec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("tool name is required")
	}
	if strings.TrimSpace(s.Description) == "" {
		return fmt.Errorf("tool %s description is required", s.Name)
	}
	if err := s.InputSchema.Validate(); err != nil {
		return fmt.Errorf("tool %s input schema: %w", s.Name, err)
	}
	if err := s.OutputSchema.Validate(); err != nil {
		return fmt.Errorf("tool %s output schema: %w", s.Name, err)
	}
	return nil
}

func ensureAllowed(spec Spec, opts ExecuteOptions) error {
	if matchesPermission(spec, opts.Forbidden) {
		return fmt.Errorf("tool %s is forbidden for this specialist", spec.Name)
	}
	if !opts.RequireListed {
		return nil
	}
	if matchesPermission(spec, opts.Allowed) {
		return nil
	}
	return fmt.Errorf("tool %s is not allowed for this specialist", spec.Name)
}

func matchesPermission(spec Spec, entries []string) bool {
	if len(entries) == 0 {
		return false
	}
	candidates := []string{spec.Name, namespacePrefix(spec.Name)}
	candidates = append(candidates, spec.Aliases...)
	for _, entry := range entries {
		entry = normalizeName(entry)
		if entry == "" {
			continue
		}
		for _, candidate := range candidates {
			if normalizeName(candidate) == entry {
				return true
			}
		}
	}
	return false
}

func namespacePrefix(name string) string {
	name = normalizeName(name)
	prefix, _, ok := strings.Cut(name, ".")
	if !ok {
		return name
	}
	return prefix
}

func normalizeName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
