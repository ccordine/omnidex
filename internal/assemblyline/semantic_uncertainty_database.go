package assemblyline

func databaseSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkDatabaseSchemaRelationChoice:
		contract = semanticUncertaintyContract(kind,
			"Which one remaining relation, if any, is necessary to answer the exact database objective?",
			"Schema structure cannot determine which semantically described relation the free-form objective still requires.",
			"Only the exact database objective, compact context, remaining relation descriptions without their identifiers, and the no-additional-relation alternative.",
			"One call-local opaque letter selecting a remaining relation description or the no-additional-relation alternative.",
			"DecodeDatabaseSchemaRelationChoiceResult maps the letter to code-owned state; code retains and removes a selected relation before independently deciding whether another round is needed.")
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
	case WorkDatabaseQueryPurposeInventory:
		contract = semanticUncertaintyContract(kind,
			"What bounded source-ordered candidate-purpose inventory is expressed for this one focused query collection?",
			"Natural-language evidence needs do not expose semantically separable collection purposes through syntax or schema structure alone.",
			"The exact evidence need, compact context, accepted query anchor and shape, and only the code-owned collection focus.",
			"One bounded raw-line inventory of untrusted candidate purposes, or the exact absence value when no purpose is expressed.",
			"DecodeDatabaseQueryPurposeInventory parses and counts the candidates; code alone owns exact filtering, source order, queue exhaustion, and required query-shape validation.")
	case WorkDatabaseQueryPurposeNecessity:
		contract = semanticUncertaintyContract(kind,
			"Is this exact candidate purpose necessary and authorized for the focused query collection?",
			"Semantic entailment from a free-form evidence need cannot be proven by query validation or text shape.",
			"Only the exact evidence need, compact context, code-owned collection focus, and one inventory-bound candidate purpose.",
			"One candidate-bound necessary-or-not-necessary relation.",
			"DecodeDatabaseQueryPurposeNecessityResult validates the relation before code skips the candidate or retains it for duplicate comparison.")
	case WorkDatabaseQueryPurposeRelation:
		contract = semanticUncertaintyContract(kind,
			"Do one candidate purpose and one already accepted purpose express the same responsibility within this query collection?",
			"Paraphrased semantic equivalence cannot be established by exact byte comparison.",
			"Only the focused collection name, one candidate purpose, and one accepted purpose.",
			"One pairwise same-or-distinct purpose relation.",
			"DecodeDatabaseQueryPurposeRelationResult validates the relation before code skips a semantic duplicate or appends one distinct accepted purpose; accepted purposes are never reopened.")
	case WorkDatabaseQueryProjectionAggregate:
		contract = semanticUncertaintyContract(kind,
			"Which registered aggregate operation applies to the next projection?",
			"The evidence need does not structurally encode whether its next value is aggregated.",
			"Only the focused projection purpose, query anchor, semantic fields, and operations with compatible projected fields.",
			"One registered projection-aggregate value.",
			"DecodeDatabaseQueryProjectionAggregateLeaf validates the value before code constrains the focused projection.")
	case WorkDatabaseQueryProjectionField:
		contract = semanticUncertaintyContract(kind,
			"Which projected field supplies the next focused projection?",
			"Field types establish compatibility but not which semantic value answers the evidence need.",
			"Only the focused projection purpose, query anchor, selected aggregate, and eligible field candidates.",
			"One opaque projection-field ID.",
			"DecodeDatabaseQueryProjectionFieldLeaf validates the ID before code binds the focused projection field.")
	case WorkDatabaseQueryProjectionTimeBucket:
		contract = semanticUncertaintyContract(kind,
			"Which registered time bucket applies to the focused temporal projection?",
			"Temporal type validation cannot infer the intended reporting granularity.",
			"Only the focused projection purpose, selected temporal field, and available bucket descriptions.",
			"One registered projection time-bucket value.",
			"DecodeDatabaseQueryProjectionTimeBucketLeaf validates the value before code binds temporal projection granularity.")
	case WorkDatabaseQueryFilterField:
		contract = semanticUncertaintyContract(kind,
			"Which projected field is constrained by the next focused filter?",
			"Eligible field structure does not identify the semantic subject of a requested constraint.",
			"Only the focused filter purpose, its parent purpose when needed, the applicable relation scope, and eligible fields.",
			"One opaque filter-field ID.",
			"DecodeDatabaseQueryFilterFieldLeaf validates the ID before code binds the focused filter field.")
	case WorkDatabaseQueryFilterOperator:
		contract = semanticUncertaintyContract(kind,
			"Which registered comparison relation applies to the focused filter field?",
			"Field type narrows valid operators but cannot determine the intended comparison meaning.",
			"Only the focused filter purpose, selected field, its parent purpose when needed, and compatible comparison descriptions.",
			"One registered filter-operator value.",
			"DecodeDatabaseQueryFilterOperatorLeaf validates the value before code binds the focused comparison.")
	case WorkDatabaseQueryFilterValue:
		contract = semanticUncertaintyContract(kind,
			"What exact literal value is required next by the focused filter?",
			"The intended literal is semantic request content not recoverable from field type alone.",
			"Only the focused literal purpose, selected field, accepted comparison, its parent purpose when needed, and any remaining closed-choice values; retained values remain in code.",
			"One exact filter-literal value.",
			"DecodeDatabaseQueryFilterValueLeaf parses the literal before code appends it to the focused filter.")
	case WorkDatabaseQueryWindowField:
		contract = semanticUncertaintyContract(kind,
			"Which temporal field is constrained by the next relative window?",
			"Temporal field eligibility does not identify the semantically intended time axis.",
			"Only the focused window purpose, query anchor, and eligible temporal fields.",
			"One opaque temporal-window field ID.",
			"DecodeDatabaseQueryWindowFieldLeaf validates the ID before code binds the focused window field.")
	case WorkDatabaseQueryWindowUnit:
		contract = semanticUncertaintyContract(kind,
			"Which registered relative time unit applies to the focused temporal field?",
			"Temporal field type cannot determine the granularity expressed by natural-language intent.",
			"Only the focused window purpose, selected temporal field, and compatible relative-time units.",
			"One registered relative-time unit.",
			"DecodeDatabaseQueryWindowUnitLeaf validates the unit before code requests its bounded amount.")
	case WorkDatabaseQueryWindowAmount:
		contract = semanticUncertaintyContract(kind,
			"What positive integer amount applies to the accepted relative-time unit?",
			"The requested duration magnitude is semantic content not derivable from temporal types.",
			"Only the focused window purpose, selected temporal field, and accepted relative-time unit.",
			"One bounded positive window amount.",
			"DecodeDatabaseQueryWindowAmountLeaf parses the amount before code binds the relative window.")
	case WorkDatabaseQueryExistenceRelation:
		contract = semanticUncertaintyContract(kind,
			"Which projected relation has its row existence tested by the next predicate?",
			"Reachable relation structure does not identify the semantic subject of an existence condition.",
			"Only the focused existence purpose, query anchor, and remaining eligible relation candidates; prior predicates stay in code.",
			"One opaque existence-relation ID.",
			"DecodeDatabaseQueryExistenceRelationLeaf validates the ID before code focuses the next predicate.")
	case WorkDatabaseQueryExistenceNegated:
		contract = semanticUncertaintyContract(kind,
			"Must rows in the focused relation exist or not exist?",
			"Schema structure cannot distinguish positive from negative natural-language existence intent.",
			"Only the focused existence purpose, selected relation, and positive-or-negative existence alternatives.",
			"One registered existence-polarity value.",
			"DecodeDatabaseQueryExistenceNegatedLeaf validates the value before code binds predicate polarity.")
	case WorkDatabaseQueryHavingAggregate:
		contract = semanticUncertaintyContract(kind,
			"Which registered aggregate is measured by the next having predicate?",
			"Several aggregate forms are structurally valid while only one matches the requested measure.",
			"Only the focused predicate purpose, query anchor, semantic fields, and aggregate operations with compatible projected fields.",
			"One registered having-aggregate value.",
			"DecodeDatabaseQueryHavingAggregateLeaf validates the value before code focuses the predicate measure.")
	case WorkDatabaseQueryHavingField:
		contract = semanticUncertaintyContract(kind,
			"Which projected field is measured by the accepted having aggregate?",
			"Aggregate compatibility does not identify the semantically intended field.",
			"Only the focused predicate purpose, query anchor, selected aggregate, and eligible field candidates.",
			"One opaque having-field ID.",
			"DecodeDatabaseQueryHavingFieldLeaf validates the ID before code binds the predicate measure.")
	case WorkDatabaseQueryHavingOperator:
		contract = semanticUncertaintyContract(kind,
			"Which registered numeric comparison applies to the focused having measure?",
			"Numeric compatibility cannot determine the comparison relation expressed by the evidence need.",
			"Only the focused predicate purpose, selected aggregate measure, and numeric comparison descriptions.",
			"One registered having-operator value.",
			"DecodeDatabaseQueryHavingOperatorLeaf validates the value before code binds the predicate comparison.")
	case WorkDatabaseQueryHavingValue:
		contract = semanticUncertaintyContract(kind,
			"What exact numeric literal is compared by the focused having predicate?",
			"The comparison threshold is semantic request content not derivable from numeric type.",
			"Only the focused predicate purpose, selected aggregate measure, and accepted comparison operator.",
			"One exact numeric having value.",
			"DecodeDatabaseQueryHavingValueLeaf parses the number before code binds the predicate threshold.")
	case WorkDatabaseQueryOrderProjection:
		contract = semanticUncertaintyContract(kind,
			"Which accepted projection is ordered by the next ordering term?",
			"Available projection positions do not identify the semantic sort key.",
			"Only the focused ordering purpose and remaining eligible projection descriptions; prior ordering terms stay in code.",
			"One opaque projection index.",
			"DecodeDatabaseQueryOrderProjectionLeaf validates the index before code focuses the ordering term.")
	case WorkDatabaseQueryOrderDirection:
		contract = semanticUncertaintyContract(kind,
			"Which registered direction applies to the focused ordered projection?",
			"Sort direction intent cannot be inferred from projection type or position.",
			"Only the focused ordering purpose, selected projection, and direction alternatives.",
			"One registered ordering-direction value.",
			"DecodeDatabaseQueryOrderDirectionLeaf validates the value before code binds the ordering term.")
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
