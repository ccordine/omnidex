package worker

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// These tests exercise the isolated compiler/test primitive. Workspace
// publication is exercised separately through the production session boundary.
func runIsolatedCompiledLanguageFixtureLifecycle(session *directCodingSession, program *directCodingProgram) error {
	generator, err := program.Project.Stack.NewSourceGenerator(session, *program)
	if err != nil {
		return err
	}
	err = runDirectCodingApplicationTaskLifecycle(program.Workload, program, directCodingApplicationTaskLifecycleHooks{
		BuildBlock: generator.GenerateBlock, VerifyTask: generator.VerifyTask, FinalStage: generator.VerifyFinal,
		PublishTask: func(*directCodingProgram, *directCodingProgram) error { return nil },
	})
	return errors.Join(err, generator.Close())
}

type compiledLanguageBehaviorFixture struct {
	requirement, implementation, verification string
}

// This constructs framework test data; it is not production intent evidence.
func compiledLanguageBehaviorWorkloadFixture(t *testing.T, stackID string, fixtures []compiledLanguageBehaviorFixture, capabilities directCodingCapabilityGraph) directCodingProgram {
	t.Helper()
	project := testCompiledLanguageVerificationProgram(t, stackID).Project
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceCommandLine, ProductQuote: "one command-line operation",
	}
	for index, fixture := range fixtures {
		specification.Requirements = append(specification.Requirements, assemblyline.Requirement{ID: fmt.Sprintf("requirement_%03d", index+1), SourceQuote: fixture.requirement})
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	target, coverage, err := resolveDirectCodingTargetTree(specification, workload, project.Stack, nil, directCodingTargetTreeOccupation{})
	if err != nil {
		t.Fatal(err)
	}
	program, err := compileDirectCodingProgram(specification, workload, capabilities, project, target, coverage, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var requirements []assemblyline.ApplicationRequirement
	byTask := make(map[string]compiledLanguageBehaviorFixture, len(fixtures))
	for index, requirement := range specification.Requirements {
		requirements = append(requirements, compiledLanguageBehaviorRequirementFixture(t, requirement))
		byTask[workload.Tasks[index].ID] = fixtures[index]
	}
	program.RequirementRelations, err = newDirectCodingApplicationTaskResultRelationPlan(workload, requirements)
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if !block.Generated() {
				continue
			}
			fixture := byTask[block.TaskID]
			body := fixture.implementation
			if block.Role == assemblyline.SourceBlockTaskVerification {
				body = fixture.verification
			}
			ref := assemblyline.SourceBlockRef{Document: document, Block: block}
			adapter, err := directCodingArtifactAdapterByID(document.AdapterID)
			if err != nil {
				t.Fatal(err)
			}
			input, err := directCodingLanguageFragmentInput(&program, ref, adapter.SourceLanguage)
			if err != nil {
				t.Fatal(err)
			}
			declaration, err := adapter.ValidateFragment(input, body)
			if err != nil {
				t.Fatalf("fixture body failed ordinary scope checks: %v", err)
			}
			program.Generated[block.ID] = declaration
		}
	}
	return program
}

func compiledLanguageBehaviorRequirementFixture(t *testing.T, requirement assemblyline.Requirement) assemblyline.ApplicationRequirement {
	t.Helper()
	authority := directCodingResultRelationAuthorityFixture(t, requirement.SourceQuote)
	derived, err := assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(assemblyline.ApplicationRequirementCandidateResultPresenceInput{
		Candidate: authority.Candidate, Kind: authority.Kind, Cardinality: authority.Cardinality,
		Dimension: assemblyline.ApplicationRequirementDerivedValueDimension,
	}, "A")
	if err != nil {
		t.Fatal(err)
	}
	determining, err := assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(assemblyline.ApplicationRequirementCandidateResultPresenceInput{
		Candidate: authority.Candidate, Kind: authority.Kind, Cardinality: authority.Cardinality,
		Dimension: assemblyline.ApplicationRequirementDeterminingRelationDimension, DerivedValuePresence: &derived,
	}, "A")
	if err != nil {
		t.Fatal(err)
	}
	relation, err := assemblyline.ResolveApplicationRequirementCandidateResultRelation(authority, derived, &determining)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationRequirement{ID: requirement.ID, Statement: requirement.SourceQuote, ResultRelation: relation}
}

type compiledLanguageBehaviorFixtureExecutor struct {
	directCodingProjectSourceGenerator
	declarations map[string]string
}

func (executor *compiledLanguageBehaviorFixtureExecutor) GenerateBlock(_ assemblyline.ApplicationTaskContext, _ *directCodingProgram, ref assemblyline.SourceBlockRef) (string, error) {
	return executor.declarations[ref.Block.ID], nil
}
