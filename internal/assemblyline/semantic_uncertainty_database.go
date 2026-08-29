package assemblyline

func databaseSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkDatabaseSchemaSelectionCoverage:
		contract = semanticUncertaintyContract(kind,
			"Does one not-yet-selected relation remain necessary for the exact evidence need?",
			"Schema structure cannot establish semantic sufficiency for a free-form evidence need.",
			"The exact evidence need, compact context, bounded relation summaries, retained relation IDs, and selection limit.",
			"One registered schema-relation coverage value.",
			"DecodeDatabaseSchemaSelectionCoverageLeaf validates the value before code continues or closes the bounded relation set.")
	case WorkDatabaseSchemaRelationSelection:
		contract = semanticUncertaintyContract(kind,
			"Which not-yet-selected relation is most necessary for the exact evidence need?",
			"Relation labels expose meaning that structural foreign-key analysis cannot rank semantically.",
			"The exact evidence need, compact context, bounded relation summaries, and retained relation IDs.",
			"One opaque database-relation ID.",
			"DecodeDatabaseSchemaRelationSelectionLeaf validates the ID before code retains the corresponding relation authority.")
	case WorkDatabaseQueryFromRelation:
		contract = semanticUncertaintyContract(kind,
			"Which projected relation should anchor the query for the exact evidence need?",
			"Several relations can be structurally reachable while only one semantically anchors the requested evidence.",
			"The exact evidence need, compact context, and code-projected relation candidates.",
			"One opaque query-anchor relation ID.",
			"DecodeDatabaseQueryFromRelationLeaf validates the ID before code binds the query anchor.")
	case WorkDatabaseQueryShape:
		contract = semanticUncertaintyContract(kind,
			"Which registered result shape directly answers the exact evidence need?",
			"Valid relational operations do not mechanically determine the user's intended answer shape.",
			"The exact evidence need, compact context, accepted anchor relation, and registered result shapes.",
			"One registered database-result shape.",
			"DecodeDatabaseQueryShapeLeaf validates the shape before code opens its compatible query leaves.")
	case WorkDatabaseQueryProjectionCoverage:
		contract = semanticUncertaintyContract(kind,
			"Is another projection required to answer the exact evidence need?",
			"Projection sufficiency is a semantic coverage judgment not derivable from query validity.",
			"The exact evidence need, compact context, accepted query shape, and retained projections.",
			"One registered projection-coverage relation.",
			"DecodeDatabaseQueryProjectionCoverageLeaf validates the relation before code continues or closes the projection set.")
	case WorkDatabaseQueryProjectionAggregate:
		contract = semanticUncertaintyContract(kind,
			"Which registered aggregate operation applies to the next projection?",
			"The evidence need does not structurally encode whether its next value is aggregated.",
			"The exact evidence need, compact context, accepted query shape, retained projections, and semantic fields.",
			"One registered projection-aggregate value.",
			"DecodeDatabaseQueryProjectionAggregateLeaf validates the value before code constrains the focused projection.")
	case WorkDatabaseQueryProjectionField:
		contract = semanticUncertaintyContract(kind,
			"Which projected field supplies the next focused projection?",
			"Field types establish compatibility but not which semantic value answers the evidence need.",
			"The exact evidence need, compact context, retained projections, focused aggregate, and eligible field candidates.",
			"One opaque projection-field ID.",
			"DecodeDatabaseQueryProjectionFieldLeaf validates the ID before code binds the focused projection field.")
	case WorkDatabaseQueryProjectionTimeBucket:
		contract = semanticUncertaintyContract(kind,
			"Which registered time bucket applies to the focused temporal projection?",
			"Temporal type validation cannot infer the intended reporting granularity.",
			"The exact evidence need, compact context, focused temporal field, and registered bucket values.",
			"One registered projection time-bucket value.",
			"DecodeDatabaseQueryProjectionTimeBucketLeaf validates the value before code binds temporal projection granularity.")
	case WorkDatabaseQueryFilterCoverage:
		contract = semanticUncertaintyContract(kind,
			"Is another filter required in the focused query scope?",
			"Query validity cannot establish semantic filter sufficiency for the evidence need.",
			"The exact evidence need, compact context, focused filter scope, and retained filters.",
			"One registered filter-coverage relation.",
			"DecodeDatabaseQueryFilterCoverageLeaf validates the relation before code continues or closes the focused filter set.")
	case WorkDatabaseQueryFilterField:
		contract = semanticUncertaintyContract(kind,
			"Which projected field is constrained by the next focused filter?",
			"Eligible field structure does not identify the semantic subject of a requested constraint.",
			"The exact evidence need, compact context, focused filter scope, retained filters, and eligible fields.",
			"One opaque filter-field ID.",
			"DecodeDatabaseQueryFilterFieldLeaf validates the ID before code binds the focused filter field.")
	case WorkDatabaseQueryFilterOperator:
		contract = semanticUncertaintyContract(kind,
			"Which registered comparison relation applies to the focused filter field?",
			"Field type narrows valid operators but cannot determine the intended comparison meaning.",
			"The exact evidence need, compact context, focused field semantics, and compatible operators.",
			"One registered filter-operator value.",
			"DecodeDatabaseQueryFilterOperatorLeaf validates the value before code binds the focused comparison.")
	case WorkDatabaseQueryFilterValueCoverage:
		contract = semanticUncertaintyContract(kind,
			"Does another distinct literal remain required by the focused set-membership filter?",
			"Semantic set membership cannot be proven complete from literal syntax or count.",
			"The exact evidence need, focused field, accepted operator, and retained literal values.",
			"One registered filter-value coverage relation.",
			"DecodeDatabaseQueryFilterValueCoverageLeaf validates the relation before code continues or closes the bounded literal set.")
	case WorkDatabaseQueryFilterValue:
		contract = semanticUncertaintyContract(kind,
			"What exact literal value is required next by the focused filter?",
			"The intended literal is semantic request content not recoverable from field type alone.",
			"The exact evidence need, focused field, accepted operator, and retained literal values.",
			"One exact filter-literal value.",
			"DecodeDatabaseQueryFilterValueLeaf parses the literal before code appends it to the focused filter.")
	case WorkDatabaseQueryWindowCoverage:
		contract = semanticUncertaintyContract(kind,
			"Is another relative temporal window required to answer the exact evidence need?",
			"Temporal-window sufficiency cannot be derived from available temporal fields.",
			"The exact evidence need, compact context, accepted query, and retained temporal windows.",
			"One registered temporal-window coverage relation.",
			"DecodeDatabaseQueryWindowCoverageLeaf validates the relation before code continues or closes the window set.")
	case WorkDatabaseQueryWindowField:
		contract = semanticUncertaintyContract(kind,
			"Which temporal field is constrained by the next relative window?",
			"Temporal field eligibility does not identify the semantically intended time axis.",
			"The exact evidence need, compact context, retained windows, and eligible temporal fields.",
			"One opaque temporal-window field ID.",
			"DecodeDatabaseQueryWindowFieldLeaf validates the ID before code binds the focused window field.")
	case WorkDatabaseQueryWindowUnit:
		contract = semanticUncertaintyContract(kind,
			"Which registered relative time unit applies to the focused temporal field?",
			"Temporal field type cannot determine the granularity expressed by natural-language intent.",
			"The exact evidence need, focused temporal field, and registered relative-time units.",
			"One registered relative-time unit.",
			"DecodeDatabaseQueryWindowUnitLeaf validates the unit before code requests its bounded amount.")
	case WorkDatabaseQueryWindowAmount:
		contract = semanticUncertaintyContract(kind,
			"What positive integer amount applies to the accepted relative-time unit?",
			"The requested duration magnitude is semantic content not derivable from temporal types.",
			"The exact evidence need, focused temporal field, and accepted relative-time unit.",
			"One bounded positive window amount.",
			"DecodeDatabaseQueryWindowAmountLeaf parses the amount before code binds the relative window.")
	case WorkDatabaseQueryExistenceCoverage:
		contract = semanticUncertaintyContract(kind,
			"Is another existence predicate required to answer the exact evidence need?",
			"Relationship reachability cannot establish semantic predicate sufficiency.",
			"The exact evidence need, compact context, accepted query, and retained existence predicates.",
			"One registered existence-predicate coverage relation.",
			"DecodeDatabaseQueryExistenceCoverageLeaf validates the relation before code continues or closes the predicate set.")
	case WorkDatabaseQueryExistenceRelation:
		contract = semanticUncertaintyContract(kind,
			"Which projected relation has its row existence tested by the next predicate?",
			"Reachable relation structure does not identify the semantic subject of an existence condition.",
			"The exact evidence need, compact context, retained predicates, and eligible relation candidates.",
			"One opaque existence-relation ID.",
			"DecodeDatabaseQueryExistenceRelationLeaf validates the ID before code focuses the next predicate.")
	case WorkDatabaseQueryExistenceNegated:
		contract = semanticUncertaintyContract(kind,
			"Must rows in the focused relation exist or not exist?",
			"Schema structure cannot distinguish positive from negative natural-language existence intent.",
			"The exact evidence need, compact context, and focused relation semantics.",
			"One registered existence-polarity value.",
			"DecodeDatabaseQueryExistenceNegatedLeaf validates the value before code binds predicate polarity.")
	case WorkDatabaseQueryHavingCoverage:
		contract = semanticUncertaintyContract(kind,
			"Is another aggregate having predicate required to answer the exact evidence need?",
			"Aggregate query validity cannot establish semantic having-predicate sufficiency.",
			"The exact evidence need, compact context, accepted projections, and retained having predicates.",
			"One registered having-predicate coverage relation.",
			"DecodeDatabaseQueryHavingCoverageLeaf validates the relation before code continues or closes the having set.")
	case WorkDatabaseQueryHavingAggregate:
		contract = semanticUncertaintyContract(kind,
			"Which registered aggregate is measured by the next having predicate?",
			"Several aggregate forms are structurally valid while only one matches the requested measure.",
			"The exact evidence need, compact context, retained having predicates, and compatible aggregates.",
			"One registered having-aggregate value.",
			"DecodeDatabaseQueryHavingAggregateLeaf validates the value before code focuses the predicate measure.")
	case WorkDatabaseQueryHavingField:
		contract = semanticUncertaintyContract(kind,
			"Which projected field is measured by the accepted having aggregate?",
			"Aggregate compatibility does not identify the semantically intended field.",
			"The exact evidence need, compact context, accepted aggregate, and eligible field candidates.",
			"One opaque having-field ID.",
			"DecodeDatabaseQueryHavingFieldLeaf validates the ID before code binds the predicate measure.")
	case WorkDatabaseQueryHavingOperator:
		contract = semanticUncertaintyContract(kind,
			"Which registered numeric comparison applies to the focused having measure?",
			"Numeric compatibility cannot determine the comparison relation expressed by the evidence need.",
			"The exact evidence need, focused aggregate measure, and registered numeric operators.",
			"One registered having-operator value.",
			"DecodeDatabaseQueryHavingOperatorLeaf validates the value before code binds the predicate comparison.")
	case WorkDatabaseQueryHavingValue:
		contract = semanticUncertaintyContract(kind,
			"What exact numeric literal is compared by the focused having predicate?",
			"The comparison threshold is semantic request content not derivable from numeric type.",
			"The exact evidence need, focused aggregate measure, and accepted comparison operator.",
			"One exact numeric having value.",
			"DecodeDatabaseQueryHavingValueLeaf parses the number before code binds the predicate threshold.")
	case WorkDatabaseQueryOrderCoverage:
		contract = semanticUncertaintyContract(kind,
			"Is another ordering term required to answer the exact evidence need?",
			"Projection structure cannot establish semantic ordering sufficiency.",
			"The exact evidence need, compact context, accepted projections, and retained ordering terms.",
			"One registered ordering-term coverage relation.",
			"DecodeDatabaseQueryOrderCoverageLeaf validates the relation before code continues or closes the ordering set.")
	case WorkDatabaseQueryOrderProjection:
		contract = semanticUncertaintyContract(kind,
			"Which accepted projection is ordered by the next ordering term?",
			"Available projection positions do not identify the semantic sort key.",
			"The exact evidence need, compact context, accepted projections, and retained ordering terms.",
			"One opaque projection index.",
			"DecodeDatabaseQueryOrderProjectionLeaf validates the index before code focuses the ordering term.")
	case WorkDatabaseQueryOrderDirection:
		contract = semanticUncertaintyContract(kind,
			"Which registered direction applies to the focused ordered projection?",
			"Sort direction intent cannot be inferred from projection type or position.",
			"The exact evidence need and focused accepted projection.",
			"One registered ordering-direction value.",
			"DecodeDatabaseQueryOrderDirectionLeaf validates the value before code binds the ordering term.")
	case WorkDatabaseEvidenceGap:
		contract = semanticUncertaintyContract(kind,
			"What single required piece of information remains unestablished by the database evidence?",
			"Structural query evidence cannot prove semantic satisfaction of every natural-language claim.",
			"The exact requirement, compact objective context, and bounded database evidence text.",
			"One optional missing-information leaf.",
			"DecodeDatabaseEvidenceGapDecision validates the leaf before code either records the gap or accepts evidence sufficiency.")
	case WorkDatabaseJoinPathSelection:
		contract = semanticUncertaintyContract(kind,
			"Which projected foreign-key path matches the exact evidence need?",
			"Referential reachability is mechanical but the intended relationship meaning is not.",
			"The exact evidence need, compact context, and code-enumerated foreign-key path candidates.",
			"One opaque database join-path ID.",
			"DecodeDatabaseJoinPathSelectionDecision validates the ID before code constructs the relational join sequence.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
