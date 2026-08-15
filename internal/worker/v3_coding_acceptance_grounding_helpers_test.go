package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingGroundingFixtureProgram(
	t *testing.T,
	product string,
	requirement string,
	criteria []string,
	acceptanceBody string,
) directCodingProgram {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: product,
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: requirement,
		}},
	}
	input := applicationWorkloadInput(specification)
	frozen, err := assemblyline.FreezeApplicationWorkload(input, assemblyline.ApplicationWorkloadDraft{
		Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
		Tasks: []assemblyline.ApplicationWorkloadTaskDraft{{
			RequirementID: "requirement_001",
			Objective:     "Implement observable behavior for " + requirement + ".",
			RequiredBehaviors: []string{
				"Expose the requested behavior through an accessible browser surface.",
			},
			AcceptanceCriteria: append([]string(nil), criteria...),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	program, err := compileDirectCodingProgram(
		"unseen", specification, nil, genericBrowserSkillBindings(specification), frozen,
		genericBrowserCapabilityBindings(specification),
	)
	if err != nil {
		t.Fatal(err)
	}
	feature, exists := directCodingTypeScriptBlueprintBlock(program.TypeScript, "feature.001")
	if !exists {
		t.Fatal("feature.001 is missing")
	}
	acceptance, exists := directCodingTypeScriptBlueprintBlock(program.TypeScript, "acceptance.001")
	if !exists {
		t.Fatal("acceptance.001 is missing")
	}
	program.Generated[feature.ID] = feature.Signature + " { return <section />; }"
	program.Generated[acceptance.ID] = acceptance.Signature + " { " + acceptanceBody + " }"
	return program
}

func directCodingGroundingInput(
	t *testing.T,
	program directCodingProgram,
	acceptanceID string,
) assemblyline.ApplicationAcceptanceGroundingReviewInput {
	t.Helper()
	context, _, recognized, err := directCodingAcceptanceTaskAuthority(program, acceptanceID)
	if err != nil {
		t.Fatal(err)
	}
	if !recognized {
		t.Fatalf("unrecognized acceptance %s", acceptanceID)
	}
	input, err := assemblyline.NewApplicationAcceptanceGroundingReviewInput(
		context, strings.TrimSpace(program.Generated[acceptanceID]),
		directCodingTypeScriptBlockIsTSX(program.TypeScript, acceptanceID),
		directCodingBrowserAcceptancePlatformAuthorities(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func directCodingAcceptedGroundingJSON(
	t *testing.T,
	input assemblyline.ApplicationAcceptanceGroundingReviewInput,
) string {
	t.Helper()
	return directCodingGroundingMatrixJSON(t, input, func(string, string) bool { return true })
}

func directCodingGroundingMatrixJSON(
	t *testing.T,
	input assemblyline.ApplicationAcceptanceGroundingReviewInput,
	value func(siteID string, criterionID string) bool,
) string {
	t.Helper()
	matrix := make(map[string]bool)
	for _, site := range input.Inventory.Sites {
		if directCodingGroundingSiteIsPlatformOnly(site) {
			continue
		}
		for _, criterion := range input.Criteria {
			matrix[site.ID+"__"+criterion.ID] = value(site.ID, criterion.ID)
		}
	}
	raw, err := json.Marshal(matrix)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func directCodingGroundingMatrixFields(
	input assemblyline.ApplicationAcceptanceGroundingReviewInput,
) []string {
	fields := make([]string, 0)
	for _, site := range input.Inventory.Sites {
		if directCodingGroundingSiteIsPlatformOnly(site) {
			continue
		}
		for _, criterion := range input.Criteria {
			fields = append(fields, site.ID+"__"+criterion.ID)
		}
	}
	return fields
}

func directCodingGroundingRaw(t *testing.T, values map[string]bool) string {
	t.Helper()
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func directCodingGroundingSiteIsPlatformOnly(site assemblyline.AcceptanceObservationSite) bool {
	if len(site.Operations) == 0 {
		return false
	}
	for _, operation := range site.Operations {
		if !strings.HasPrefix(operation, "harness_call:") {
			return false
		}
	}
	return true
}

func directCodingPortableCandidate(job assemblyline.PortableJob, candidate string) assemblyline.PortableResult {
	return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}
}

func directCodingSourceLocation(t *testing.T, source string, needle string) (int, int) {
	t.Helper()
	offset := strings.Index(source, needle)
	if offset < 0 {
		t.Fatalf("source omits location needle %q", needle)
	}
	prefix := source[:offset]
	line := strings.Count(prefix, "\n") + 1
	lineStart := strings.LastIndex(prefix, "\n") + 1
	return line, offset - lineStart + 1
}

func directCodingGroundingSiteIDForOperation(
	t *testing.T,
	input assemblyline.ApplicationAcceptanceGroundingReviewInput,
	operation string,
) string {
	t.Helper()
	for _, site := range input.Inventory.Sites {
		for _, actual := range site.Operations {
			if actual == operation {
				return site.ID
			}
		}
	}
	t.Fatalf("grounding inventory omits operation %s: %+v", operation, input.Inventory.Sites)
	return ""
}
