package assemblyline

import "testing"

func TestPortableResponseTransportRegistryIsExhaustiveAndRaw(t *testing.T) {
	seen := make(map[WorkKind]struct{}, len(AllWorkKinds()))
	for _, kind := range AllWorkKinds() {
		if _, duplicate := seen[kind]; duplicate {
			t.Fatalf("portable work kind %q is registered twice", kind)
		}
		seen[kind] = struct{}{}

		transport, err := PortableResponseTransportForWorkKind(kind)
		if err != nil {
			t.Fatalf("transport for %q: %v", kind, err)
		}
		scope, err := PortableWorkerScopeForWorkKind(kind)
		if err != nil {
			t.Fatalf("scope for %q: %v", kind, err)
		}

		wantTransport := PortableResponseTransportSemanticRaw
		wantScope := PortableSemanticWorkerScope
		switch kind {
		case WorkApplicationTargetTree:
			wantTransport = PortableResponseTransportStructuralRaw
			wantScope = PortableStructuralWorkerScope
		case WorkFragmentGeneration, WorkFragmentGenerationReplacement,
			WorkFragmentModification, WorkFragmentCorrection:
			wantTransport = PortableResponseTransportFragmentRaw
			wantScope = PortableFragmentWorkerScope
		}
		if transport != wantTransport || scope != wantScope {
			t.Fatalf(
				"transport for %q = (%q, %q), want (%q, %q)",
				kind, transport, scope, wantTransport, wantScope,
			)
		}
	}
}

func TestPortableResponseTransportRejectsUnregisteredAuthority(t *testing.T) {
	if _, err := PortableResponseTransportForWorkKind("unknown"); err == nil {
		t.Fatal("unregistered work kind received a response transport")
	}
	if _, err := PortableResponseTransport("unknown").WorkerScope(); err == nil {
		t.Fatal("unregistered response transport received a worker scope")
	}
}

func TestPortableResponseTransportRejectsRetiredAggregateKinds(t *testing.T) {
	for _, retired := range []WorkKind{
		"application_context_needs",
		"application_intent",
		"application_job_specification",
		"application_service_state_interface",
		"repository_requirements",
		"repository_change_surface",
		"repository_search_term",
		"repository_search_anchor_coverage",
		"repository_search_anchor",
		"repository_evidence_relevance",
		"repository_grounded_review",
		"repository_grounded_issue_detail",
		"repository_grounded_issue_kind",
		"repository_grounded_correction",
		"context_search_terms",
		"context_search_term_coverage",
		"context_search_term",
		"context_relevance",
		"roleplay_canon_extraction",
		"roleplay_grounded_response",
		"grounded_answer",
		"database_schema_selection",
		"database_query_intent",
		"web_search_terms",
		"web_relevance",
		"web_grounded_synthesis",
		"web_grounded_synthesis_correction",
		"web_claim_evidence_review",
		"web_review_claim_coverage",
		"web_review_claim",
		"web_review_claim_verdict",
		"web_review_issue_evidence_relation",
		"web_review_issue_detail",
	} {
		if validWorkKind(retired) {
			t.Fatalf("retired aggregate work kind %q remains registered", retired)
		}
		if _, err := PortableResponseTransportForWorkKind(retired); err == nil {
			t.Fatalf("retired aggregate work kind %q received a response transport", retired)
		}
	}
}

func TestPortableSemanticLeafKindsAreRegistered(t *testing.T) {
	registered := make(map[WorkKind]struct{}, len(AllWorkKinds()))
	for _, kind := range AllWorkKinds() {
		registered[kind] = struct{}{}
	}
	for _, kind := range []WorkKind{
		WorkApplicationContextNeedCoverage,
		WorkApplicationContextNeedQuestion,
		WorkApplicationProductContext,
		WorkApplicationRequirementCoverage,
		WorkApplicationRequirement,
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateOutcomeRelation,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationRequirementCandidateResultRelationGrounding,
		WorkApplicationRequirementCandidateResultRelationCorrection,
		WorkApplicationRequirementCandidateSplit,
		WorkApplicationRequirementCandidateSplitCorrection,
		WorkApplicationServiceEndpointExposure,
		WorkApplicationServiceEndpointMethod,
		WorkApplicationServiceEndpointRouteTemplate,
		WorkApplicationServiceEndpointRequestMedia,
		WorkApplicationServiceEndpointResponseMedia,
		WorkApplicationServiceEndpointSuccessStatus,
		WorkRepositoryRequirementCoverage,
		WorkRepositoryRequirement,
		WorkRepositoryChangeOwner,
		WorkRepositoryEvidenceRelevanceLeaf,
		WorkContextRelevanceSelection,
		WorkRoleplayCanonFactCoverage,
		WorkRoleplayCanonFact,
		WorkRoleplayGroundedResponseText,
		WorkRoleplayGroundedResponseEvidenceRelation,
		WorkGroundedAnswerText,
		WorkGroundedAnswerEvidenceRelation,
		WorkDatabaseSchemaSelectionCoverage,
		WorkDatabaseSchemaRelationSelection,
		WorkDatabaseQueryFromRelation,
		WorkDatabaseQueryShape,
		WorkDatabaseQueryProjectionCoverage,
		WorkDatabaseQueryProjectionAggregate,
		WorkDatabaseQueryProjectionField,
		WorkDatabaseQueryProjectionTimeBucket,
		WorkDatabaseQueryFilterCoverage,
		WorkDatabaseQueryFilterField,
		WorkDatabaseQueryFilterOperator,
		WorkDatabaseQueryFilterValueCoverage,
		WorkDatabaseQueryFilterValue,
		WorkDatabaseQueryWindowCoverage,
		WorkDatabaseQueryWindowField,
		WorkDatabaseQueryWindowUnit,
		WorkDatabaseQueryWindowAmount,
		WorkDatabaseQueryExistenceCoverage,
		WorkDatabaseQueryExistenceRelation,
		WorkDatabaseQueryExistenceNegated,
		WorkDatabaseQueryHavingCoverage,
		WorkDatabaseQueryHavingAggregate,
		WorkDatabaseQueryHavingField,
		WorkDatabaseQueryHavingOperator,
		WorkDatabaseQueryHavingValue,
		WorkDatabaseQueryOrderCoverage,
		WorkDatabaseQueryOrderProjection,
		WorkDatabaseQueryOrderDirection,
		WorkWebRelevanceRelation,
		WorkWebSynthesisParagraphCoverage,
		WorkWebSynthesisParagraph,
		WorkWebSynthesisEvidenceRelation,
	} {
		if _, ok := registered[kind]; !ok {
			t.Fatalf("semantic leaf work kind %q is absent from the registry", kind)
		}
		transport, err := PortableResponseTransportForWorkKind(kind)
		if err != nil || transport != PortableResponseTransportSemanticRaw {
			t.Fatalf("semantic leaf work kind %q transport=%q error=%v", kind, transport, err)
		}
	}
}

func TestNewPortableJobsUseRawTransportIdentity(t *testing.T) {
	const request = "Build a durable service."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	job, err := NewApplicationProductContextJob(ApplicationProductContextInput{
		UserRequest: request, Context: context,
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Schema != PortableJobSchemaV2 {
		t.Fatalf("portable schema=%q want %q", job.Schema, PortableJobSchemaV2)
	}
	if err := (PortableJob{
		Schema: "omnidex.portable-job.v1",
		ID:     job.ID, Kind: job.Kind, Payload: job.Payload,
	}).Validate(); err == nil {
		t.Fatal("historical structured portable authority was accepted as current")
	}
}
