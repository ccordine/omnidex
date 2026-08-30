package assemblyline

func databaseSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkDatabaseSchemaRelationInventory:
		contract = semanticUncertaintyContract(kind,
			"What bounded inventory of available relation responsibilities might be necessary for the exact database objective?",
			"Natural-language database objectives and relation descriptors cannot be semantically aligned by syntax or schema structure alone.",
			"The exact database objective, compact context, and bounded registered relation descriptors without their identifiers.",
			"One bounded raw-line inventory of candidate relation responsibilities with no selection or completion metadata.",
			"DecodeDatabaseSchemaRelationInventory binds the inventory to its authority; code alone owns the candidate queue and every subsequent disposition.")
	case WorkDatabaseSchemaRelationNecessity:
		contract = semanticUncertaintyContract(kind,
			"Is this exact candidate relation responsibility necessary for the exact database objective?",
			"Semantic necessity for a free-form database objective cannot be established by schema syntax or structural validation.",
			"Only the exact database objective, compact context, and one exact candidate relation responsibility.",
			"One candidate-bound necessary-or-not-necessary relation.",
			"DecodeDatabaseSchemaRelationNecessityResult validates the relation before code skips the suggestion or opens its independent registered-relation resolution.")
	case WorkDatabaseSchemaRelationResolution:
		contract = semanticUncertaintyContract(kind,
			"Which one registered relation supplies this exact necessary relation responsibility?",
			"Relation descriptors expose semantic meaning that opaque IDs and structural validation cannot map to a free-form responsibility.",
			"Only one exact necessary relation responsibility and the bounded registered relation IDs with their descriptors.",
			"One opaque registered database-relation ID.",
			"DecodeDatabaseSchemaRelationResolutionResult validates the ID before code skips a duplicate or retains the new selection; accepted selections are never reopened.")
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
			"One bounded raw-line inventory of untrusted candidate purposes with no parameter or completion metadata.",
			"DecodeDatabaseQueryPurposeInventory binds the inventory to its authority; code alone owns exact filtering, source order, and queue exhaustion.")
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
	case WorkDatabaseQueryFilterValue:
		contract = semanticUncertaintyContract(kind,
			"What exact literal value is required next by the focused filter?",
			"The intended literal is semantic request content not recoverable from field type alone.",
			"The exact evidence need, focused field, accepted operator, and retained literal values.",
			"One exact filter-literal value.",
			"DecodeDatabaseQueryFilterValueLeaf parses the literal before code appends it to the focused filter.")
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
