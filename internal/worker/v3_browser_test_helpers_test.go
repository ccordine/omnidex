package worker

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func testBlockDependsOn(block assemblyline.SourceBlock, dependencyID string) bool {
	for _, candidate := range block.DependsOn {
		if candidate == dependencyID {
			return true
		}
	}
	return false
}

func testTypeScriptBrowserProject(t *testing.T) (
	directCodingProjectStack,
	directCodingProjectVersionProfile,
) {
	t.Helper()
	stack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatalf("resolve browser stack: %v", err)
	}
	for _, profile := range registeredDirectCodingProjectVersionProfiles() {
		if profile.ID == typeScriptBrowserVersionProfileV1 {
			return stack, profile
		}
	}
	t.Fatalf("browser version profile %s is not registered", typeScriptBrowserVersionProfileV1)
	return directCodingProjectStack{}, directCodingProjectVersionProfile{}
}

func testBrowserManifest(
	t *testing.T,
	files []directCodingFileTask,
) typeScriptPackageManifest {
	t.Helper()
	for _, file := range files {
		if file.Path != "package.json" {
			continue
		}
		var manifest typeScriptPackageManifest
		if err := json.Unmarshal(file.Content, &manifest); err != nil {
			t.Fatalf("decode browser package manifest: %v", err)
		}
		return manifest
	}
	t.Fatal("browser static files omit package.json")
	return typeScriptPackageManifest{}
}

func testTypeScriptBrowserProgram(
	t *testing.T,
	packageName string,
	product string,
	requirement string,
) directCodingProgram {
	t.Helper()
	return testTypeScriptBrowserProgramAtRoot(
		t, packageName, product, requirement, "",
	)
}

func testTypeScriptBrowserProgramAtRoot(
	t *testing.T,
	packageName string,
	product string,
	requirement string,
	root string,
) directCodingProgram {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: product,
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: requirement,
		}},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatalf("freeze browser fixture workload: %v", err)
	}
	stack, profile := testTypeScriptBrowserProject(t)
	occupation := directCodingTargetTreeOccupation{}
	if root != "" {
		occupation, err = snapshotDirectCodingTargetTreeOccupation(root, stack)
		if err != nil {
			t.Fatalf("snapshot browser fixture target occupation: %v", err)
		}
	}
	target, coverage, err := resolveDirectCodingTargetTree(
		specification, workload, stack, nil, occupation,
	)
	if err != nil {
		t.Fatalf("resolve browser fixture target tree: %v", err)
	}
	dialect, err := directCodingProjectSourceDialect(profile)
	if err != nil {
		t.Fatalf("resolve browser fixture dialect: %v", err)
	}
	program, err := compileDirectCodingProgram(
		packageName,
		specification,
		workload,
		directCodingCapabilityGraph{"requirement_001": nil},
		directCodingProjectSelection{Stack: stack, Profile: profile, Dialect: dialect},
		target,
		coverage,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("compile browser fixture program: %v", err)
	}
	const request = "Build the supplied browser fixture."
	requestAuthority, err := newDirectCodingApplicationRequestAuthority(request, request)
	if err != nil {
		t.Fatalf("construct browser fixture request authority: %v", err)
	}
	relationAuthority := directCodingResultRelationAuthorityFixture(t, requirement)
	derived, err := assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(
		assemblyline.ApplicationRequirementCandidateResultPresenceInput{
			Candidate: relationAuthority.Candidate,
			Kind:      relationAuthority.Kind, Cardinality: relationAuthority.Cardinality,
			Dimension: assemblyline.ApplicationRequirementDerivedValueDimension,
		},
		string(assemblyline.ApplicationRequirementCandidateResultAbsent),
	)
	if err != nil {
		t.Fatalf("decode browser fixture result presence: %v", err)
	}
	receipt, err := assemblyline.ResolveApplicationRequirementCandidateResultRelation(
		relationAuthority, derived, nil,
	)
	if err != nil {
		t.Fatalf("resolve browser fixture result relation: %v", err)
	}
	program.RequirementRelations, err = newDirectCodingApplicationTaskResultRelationPlan(
		workload,
		[]assemblyline.ApplicationRequirement{{
			ID: "requirement_001", Statement: requirement,
			RequestSHA256: requestAuthority.requestSHA256, ResultRelation: receipt,
		}},
		requestAuthority,
	)
	if err != nil {
		t.Fatalf("construct browser fixture result-relation plan: %v", err)
	}
	return program
}
