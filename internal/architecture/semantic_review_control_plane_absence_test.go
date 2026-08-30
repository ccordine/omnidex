package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionHasNoRetiredSemanticReviewControlPlane(t *testing.T) {
	t.Parallel()
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	forbidden := []string{
		"WorkApplicationRequirementInventoryCardinality",
		"AT_MOST_ONE_RUNTIME_OUTCOME",
		"WorkApplicationContextNeedCoverage",
		"WorkApplicationContextNeedQuestion",
		"WorkApplicationRequirementCoverage",
		"WorkApplicationRequirementCandidateSplit",
		"WorkApplicationRequirementCandidateSplitCorrection",
		"WorkApplicationStateFieldCoverage",
		"WorkApplicationRecordFieldCoverage",
		"WorkRepositoryRequirementCoverage",
		"WorkRepositoryEvidenceRelevanceLeaf",
		"WorkContextRelevanceSelection",
		"WorkRoleplayGroundedResponseText",
		"WorkRoleplayCanonFactCoverage",
		"WorkGroundedAnswerText",
		"WorkGroundedAnswerEvidenceRelation",
		"WorkDatabaseSchemaSelectionCoverage",
		"WorkDatabaseSchemaRelationSelection",
		"WorkDatabaseQueryProjectionCoverage",
		"WorkDatabaseQueryFilterCoverage",
		"WorkDatabaseQueryFilterValueCoverage",
		"WorkDatabaseQueryWindowCoverage",
		"WorkDatabaseQueryExistenceCoverage",
		"WorkDatabaseQueryHavingCoverage",
		"WorkDatabaseQueryOrderCoverage",
		"WorkDatabaseEvidenceGap",
		"WorkDatabaseEvidenceRefinementCandidate",
		"WorkDatabaseEvidenceRefinementAuthorization",
		"DatabaseEvidenceRefinement",
		"database_evidence_refinement",
		"database-evidence-refinement",
		"WorkWebSynthesisParagraphCoverage",
		"WorkRuntimeCapabilitySelection",
		"response_correction",
		"ApplicationContextNeedRemains",
		"ApplicationNoUncoveredContextNeed",
		"ApplicationRequirementRemains",
		"ApplicationNoUncoveredRequirement",
		"ApplicationStateFieldRemains",
		"ApplicationNoUncoveredStateField",
		"ApplicationRecordFieldRemains",
		"ApplicationNoUncoveredRecordField",
		"RepositoryRequirementRemains",
		"RepositoryNoUncoveredRequirement",
		"RepositoryEvidenceNoRelevantCandidate",
		"ContextRelevanceNoCandidate",
		"RoleplayCanonFactRemains",
		"RoleplayNoUncoveredCanonFact",
		"DatabaseSchemaRelationRemains",
		"DatabaseSchemaNoUncoveredRelation",
		"DatabaseQueryItemRemains",
		"DatabaseQueryNoUncoveredItem",
		"DatabaseQueryValueRemains",
		"DatabaseQueryNoUncoveredValue",
		"WebSynthesisParagraphRemains",
		"WebSynthesisNoUncoveredParagraph",
		"RuntimeCapabilitySelectionNone",
		"CONTEXT_NEED_REMAINS",
		"NO_UNCOVERED_CONTEXT_NEED",
		"NO_RELEVANT_CANDIDATE",
		"REQUIREMENT_REMAINS",
		"NO_UNCOVERED_REQUIREMENT",
		"STATE_FIELD_REMAINS",
		"NO_UNCOVERED_STATE_FIELD",
		"RECORD_FIELD_REMAINS",
		"NO_UNCOVERED_RECORD_FIELD",
		"NO_RELEVANT_EVIDENCE",
		"CANON_FACT_REMAINS",
		"NO_UNCOVERED_CANON_FACT",
		"RELATION_REMAINS",
		"NO_UNCOVERED_RELATION",
		"ITEM_REMAINS",
		"NO_UNCOVERED_ITEM",
		"VALUE_REMAINS",
		"NO_UNCOVERED_VALUE",
		"PARAGRAPH_REMAINS",
		"NO_UNCOVERED_PARAGRAPH",
		"RUNTIME_CAPABILITY_SELECTION_NONE",
		"ALREADY_ACCEPTED_PURPOSES",
		"REMAINING_CANDIDATE_PURPOSES",
		"CODE_OWNED_CANDIDATES_JSON",
		"CODE_SELECTED_FILE_COUNT",
		"CODE_SELECTED_ROOT_FILES_ONLY",
		"CODE_RESERVED_TREE",
		"CODE_SELECTED_TECHNICAL_CONTEXT",
		"CODE_PROVEN_PUBLIC_INTERACTION_SURFACE",
		"CODE_OWNED_DIRECT_CAPABILITY_CONSUMER",
		"EXACT_OUTPUT_LIMIT_EVIDENCE",
	}

	for _, relative := range []string{"cmd", "internal"} {
		walkProductionSource(t, filepath.Join(repositoryRoot, relative), func(path, source string) {
			for _, token := range forbidden {
				if strings.Contains(source, token) {
					t.Errorf("production source %s retains retired semantic control-plane token %q", path, token)
				}
			}
		})
	}

	schemaPath := filepath.Join(repositoryRoot, "database", "setup.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		"'application_requirement_inventory_cardinality'",
		"AT_MOST_ONE_RUNTIME_OUTCOME",
		"'application_context_need_coverage'",
		"'application_context_need_question'",
		"'application_requirement_coverage'",
		"'application_requirement_candidate_split'",
		"'application_requirement_candidate_split_correction'",
		"'application_state_field_coverage'",
		"'application_record_field_coverage'",
		"'repository_requirement_coverage'",
		"'repository_evidence_relevance_leaf'",
		"'context_relevance_selection'",
		"'roleplay_grounded_response_text'",
		"'roleplay_canon_fact_coverage'",
		"'grounded_answer_text'",
		"'grounded_answer_evidence_relation'",
		"'database_schema_selection_coverage'",
		"'database_schema_relation_selection'",
		"'database_query_projection_coverage'",
		"'database_query_filter_coverage'",
		"'database_query_filter_value_coverage'",
		"'database_query_window_coverage'",
		"'database_query_existence_coverage'",
		"'database_query_having_coverage'",
		"'database_query_order_coverage'",
		"'database_evidence_gap'",
		"'database_evidence_refinement_candidate'",
		"'database_evidence_refinement_authorization'",
		"'database_evidence_refinement'",
		"'web_synthesis_paragraph_coverage'",
		"'runtime_capability_selection'",
		"'response_correction'",
	} {
		if strings.Contains(string(schema), token) {
			t.Errorf("database schema retains retired semantic control-plane token %q", token)
		}
	}

	for _, relative := range []string{".env.example", "default.env"} {
		template, err := os.ReadFile(filepath.Join(repositoryRoot, relative))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(template), "OMNI_DATABASE_EVIDENCE_REFINEMENT_MODEL") {
			t.Errorf("%s retains retired database evidence-refinement model route", relative)
		}
	}
}
