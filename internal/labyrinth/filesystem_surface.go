package labyrinth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
)

const FilesystemSurfaceVersionV1 = "filesystem.v1"

type FilesystemEnvironment struct {
	kernel  *Environment
	surface *filesystemSurface
}

var _ cognition.Environment = (*FilesystemEnvironment)(nil)

func NewFilesystemEnvironment(
	scenario Scenario,
	episode cognition.EpisodeRef,
	authorize AttemptAuthorizer,
) (*FilesystemEnvironment, error) {
	surface, err := newFilesystemSurface(scenario)
	if err != nil {
		return nil, err
	}
	kernel, err := newEnvironment(scenario, episode, authorize, surface)
	if err != nil {
		_ = surface.Close()
		return nil, err
	}
	return &FilesystemEnvironment{kernel: kernel, surface: surface}, nil
}

func (environment *FilesystemEnvironment) Start(
	ctx context.Context,
	scenario cognition.ScenarioRef,
) (cognition.Transition, error) {
	return environment.kernel.Start(ctx, scenario)
}

func (environment *FilesystemEnvironment) Apply(
	ctx context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	return environment.kernel.Apply(ctx, episode, expected, action)
}

func (environment *FilesystemEnvironment) EvaluateGoal(
	ctx context.Context,
	episode cognition.EpisodeRef,
	revision cognition.WorldRevision,
	desired cognition.GoalExpression,
) (bool, error) {
	if environment == nil || environment.kernel == nil {
		return false, fmt.Errorf("%w: filesystem environment is unavailable", cognition.ErrInvalidRevision)
	}
	return environment.kernel.EvaluateGoal(ctx, episode, revision, desired)
}

func (environment *FilesystemEnvironment) Close() error {
	environment.kernel.mu.Lock()
	defer environment.kernel.mu.Unlock()
	return environment.surface.Close()
}

func (environment *FilesystemEnvironment) MarshalJSON() ([]byte, error) {
	return json.Marshal(environment.kernel)
}

func (environment *FilesystemEnvironment) rootPath() string {
	environment.kernel.mu.Lock()
	defer environment.kernel.mu.Unlock()
	return environment.surface.root
}

func (environment *FilesystemEnvironment) surfaceStateSHA256() string {
	environment.kernel.mu.Lock()
	defer environment.kernel.mu.Unlock()
	return environment.kernel.surfaceState.StateSHA256
}

func (environment *FilesystemEnvironment) surfaceExecutionCount() uint64 {
	environment.kernel.mu.Lock()
	defer environment.kernel.mu.Unlock()
	return environment.surface.executions
}

type filesystemSurface struct {
	root       string
	rgPath     string
	stages     []EntityID
	records    []PublicRecord
	closed     bool
	executions uint64
}

func newFilesystemSurface(scenario Scenario) (*filesystemSurface, error) {
	if err := scenario.Validate(); err != nil {
		return nil, err
	}
	if scenario.artifactCorpus != nil {
		return nil, fmt.Errorf("%w: filesystem v1 does not support lazy artifact corpora", ErrSurfaceLimit)
	}
	if err := validateV1SurfaceCatalog(scenario.Catalog()); err != nil {
		return nil, err
	}
	stages := make([]EntityID, 0)
	for _, entity := range scenario.definition.entities {
		if entity.Kind == stageKind {
			if !safeSurfaceSegment(string(entity.ID)) {
				return nil, fmt.Errorf("%w: unsafe stage identity", ErrSurfaceOperation)
			}
			stages = append(stages, entity.ID)
		}
	}
	for _, record := range scenario.descriptor.Records {
		if !safeSurfaceSegment(string(record.ID)) || !safeSurfaceSegment(string(record.Location)) {
			return nil, fmt.Errorf("%w: unsafe record identity", ErrSurfaceOperation)
		}
	}
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return nil, fmt.Errorf("%w: rg --json executable is required: %v", ErrSurfaceOperation, err)
	}
	rgPath, err = filepath.Abs(rgPath)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve rg executable: %v", ErrSurfaceOperation, err)
	}
	root, err := os.MkdirTemp("", "omnidex-labyrinth-filesystem-")
	if err != nil {
		return nil, fmt.Errorf("%w: create isolated root: %v", ErrSurfaceOperation, err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return nil, fmt.Errorf("%w: secure isolated root: %v", ErrSurfaceOperation, err)
	}
	return &filesystemSurface{
		root: root, rgPath: rgPath, stages: append([]EntityID(nil), stages...),
		records: append([]PublicRecord(nil), scenario.descriptor.Records...),
	}, nil
}

func validateV1SurfaceCatalog(catalog cognition.ActionCatalog) error {
	if catalog.ID != "labyrinth.actions.v1" || catalog.Version != GrammarVersionV1 ||
		len(catalog.Schemas) != len(v1MacroKinds) {
		return fmt.Errorf("%w: v1 surface requires exactly seven action schemas", ErrSurfaceOperation)
	}
	for _, kind := range v1MacroKinds {
		schema, exists := catalog.Schema(kind)
		if !exists || !validSurfaceParameters(schema) {
			return fmt.Errorf("%w: v1 surface action %q has an incompatible schema", ErrSurfaceOperation, kind)
		}
	}
	return nil
}

func validSurfaceParameters(schema cognition.ActionSchema) bool {
	expected := map[cognition.ActionKind][]cognition.ActionArgumentName{
		"observe":  {},
		"search":   {queryArg},
		"read":     {artifactArg},
		"navigate": {fromArg, toArg},
		"take":     {objectArg},
		"use":      {itemArg, targetArg},
		"write":    {expectedSHA256Arg, mutationTargetArg, mutationValueArg},
	}[schema.Kind]
	if expected == nil && schema.Kind != "observe" {
		return false
	}
	if schema.ID != cognition.ActionSchemaID("labyrinth.action."+string(schema.Kind)+".v1") ||
		schema.Version != GrammarVersionV1 {
		return false
	}
	actual := make([]cognition.ActionArgumentName, len(schema.Parameters))
	for index, parameter := range schema.Parameters {
		if !parameter.Required || parameter.MaxBytes != cognition.MaxActionValueBytes {
			return false
		}
		actual[index] = parameter.Name
	}
	evidenceSet := false
	if len(actual) == len(expected)+1 {
		for _, name := range actual {
			evidenceSet = evidenceSet || name == evidenceSetArg
		}
	}
	want := append([]cognition.ActionArgumentName{}, expected...)
	if evidenceSet {
		want = append(want, evidenceSetArg)
	}
	sort.Slice(want, func(left, right int) bool { return want[left] < want[right] })
	expectedEvidence := cognition.EvidenceOptional
	if evidenceSet {
		expectedEvidence = cognition.EvidenceRequired
	}
	return schema.EvidencePolicy == expectedEvidence && slices.Equal(actual, want)
}

func safeSurfaceSegment(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value &&
		!strings.ContainsAny(value, `/\\`)
}

func (surface *filesystemSurface) Close() error {
	if surface.closed {
		return nil
	}
	if err := os.RemoveAll(surface.root); err != nil {
		return fmt.Errorf("%w: remove isolated root: %v", ErrSurfaceOperation, err)
	}
	surface.closed = true
	return nil
}
