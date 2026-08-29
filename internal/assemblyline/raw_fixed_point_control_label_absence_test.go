package assemblyline

import (
	"os"
	"strings"
	"testing"
	"unicode"
)

func TestRawFixedPointRelationRegistryHasNoWorkflowControlLabels(t *testing.T) {
	registry := map[string][2]string{
		"application context need": {ApplicationContextNeedRemains, ApplicationNoUncoveredContextNeed},
		"application requirement":  {ApplicationRequirementRemains, ApplicationNoUncoveredRequirement},
		"repository requirement":   {RepositoryRequirementRemains, RepositoryNoUncoveredRequirement},
		"roleplay canon fact":      {RoleplayCanonFactRemains, RoleplayNoUncoveredCanonFact},
		"database schema relation": {DatabaseSchemaRelationRemains, DatabaseSchemaNoUncoveredRelation},
		"database query item":      {DatabaseQueryItemRemains, DatabaseQueryNoUncoveredItem},
		"database query value":     {DatabaseQueryValueRemains, DatabaseQueryNoUncoveredValue},
		"web synthesis paragraph":  {string(WebSynthesisParagraphRemains), string(WebSynthesisNoUncoveredParagraph)},
	}
	forbidden := map[string]struct{}{
		"COMPLETE": {}, "COMPLETED": {}, "DONE": {}, "STOP": {}, "CONTINUE": {},
		"ACCEPT": {}, "REJECT": {}, "RETRY": {}, "REPAIR": {}, "APPLY": {},
		"EXECUTE": {}, "SEARCH": {}, "PLAN": {}, "PASS": {}, "FAIL": {},
		"SUCCESS": {}, "PROCEED": {}, "HALT": {}, "FINISH": {}, "FINISHED": {},
		"FINAL": {}, "APPROVE": {}, "APPROVED": {}, "RUN": {},
	}

	for station, alternatives := range registry {
		if !strings.HasSuffix(alternatives[0], "_REMAINS") {
			t.Fatalf("%s positive relation %q does not describe a remaining item", station, alternatives[0])
		}
		if !strings.HasPrefix(alternatives[1], "NO_UNCOVERED_") {
			t.Fatalf("%s negative relation %q does not describe absence of an uncovered item", station, alternatives[1])
		}
		for _, alternative := range alternatives {
			for _, token := range strings.FieldsFunc(alternative, func(r rune) bool {
				return !unicode.IsLetter(r) && !unicode.IsDigit(r)
			}) {
				if _, blocked := forbidden[token]; blocked {
					t.Fatalf("%s raw relation %q contains workflow-control token %q", station, alternative, token)
				}
			}
		}
	}
}

func TestRawFixedPointStationSourceHasNoRetiredControlAlternatives(t *testing.T) {
	files := []string{
		"application_context_need_leaves.go",
		"application_intent_leaves.go",
		"repository_requirement_leaves.go",
		"roleplay_canon_fact_leaves.go",
		"database_schema_selection_leaves.go",
		"database_query_intent_leaf_types.go",
		"database_query_intent_leaf_prompt.go",
		"web_grounded_synthesis_leaves.go",
	}
	retired := []string{
		"CONTEXT_NEEDS_COMPLETE", "COVERAGE_COMPLETE", "SEARCH_ANCHORS_COMPLETE",
		"CONTEXT_TERMS_COMPLETE", "CANON_FACTS_COMPLETE", "SELECTION_COMPLETE",
		"COLLECTION_COMPLETE", "VALUES_COMPLETE", "SEARCH_TERMS_COMPLETE",
		"SEARCH_ANCHOR_REMAINS", "SEARCH_TERM_REMAINS",
	}
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, label := range retired {
			if strings.Contains(string(source), label) {
				t.Fatalf("%s retains model-authored workflow label %q", file, label)
			}
		}
	}
}
