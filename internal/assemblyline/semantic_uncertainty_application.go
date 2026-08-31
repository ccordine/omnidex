package assemblyline

func applicationSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkApplicationContextQuestionInventory:
		contract = semanticUncertaintyContractV3(kind,
			"What bounded source-ordered inventory of candidate repository-fact questions, if any, may be needed to interpret the immutable request under the established context?",
			"Potential semantic dependencies between unconstrained request language and repository facts cannot be enumerated from repository syntax alone.",
			"The immutable request, established application facts, and exact candidate count and byte bounds.",
			"One bounded raw-line repository-fact-question inventory or the registered semantic absence.",
			"DecodeApplicationContextQuestionInventory parses and binds the inventory before code alone owns exact deduplication, the candidate queue, authorization, evidence resolution, accepted context, and exhaustion.")
	case WorkApplicationContextQuestionNecessity:
		contract = semanticUncertaintyContractV2(kind,
			"Is one exact inventory candidate a necessary, still-unresolved repository-fact question under the immutable request and current established context?",
			"Necessity and unresolvedness of a natural-language repository-fact dependency cannot be established by byte comparison or repository syntax.",
			"The immutable request, current established facts, and one exact inventory candidate; accepted question text is excluded.",
			"One registered necessary-unresolved or not-necessary/already-resolved relation.",
			"DecodeApplicationContextQuestionNecessityResult validates the candidate-bound relation before code discards a negative candidate immediately or admits a positive candidate to pairwise duplicate checks and one registered repository-evidence resolution.")
	case WorkApplicationContextQuestionRelation:
		contract = semanticUncertaintyContract(kind,
			"Do one exact queued repository-fact question and one exact accepted question ask for the same repository fact?",
			"Semantic identity between byte-different natural-language questions cannot be established by byte comparison or repository syntax.",
			"Exactly one queued candidate question and one immutable accepted question; no request, facts, accepted set, or queue state.",
			"One registered pair-bound same-fact or distinct-facts relation.",
			"DecodeApplicationContextQuestionRelationResult binds the pair before code discards only the duplicate candidate or compares the next accepted question; accepted state is never reopened.")
	case WorkApplicationProductContext:
		contract = semanticUncertaintyContractV2(kind,
			"What concise product or domain identity is explicitly established by the immutable software request and established facts, excluding its requirements?",
			"Product or domain identity expressed in natural language has no mechanically exact structural representation.",
			"The immutable user request and established application facts needed to identify only the product, domain, audience, setting, or purpose.",
			"One concise product-or-domain identity phrase that excludes software requirements.",
			"DecodeApplicationProductContextLeaf validates the leaf before code binds it as accepted application authority.")
	case WorkApplicationRequirementInventory:
		contract = semanticUncertaintyContractV10(kind,
			"What bounded source-ordered inventory of atomic finished-software runtime-outcome candidates is grounded by the immutable request and established facts?",
			"Atomic runtime-outcome wording and the literal core operation carried by natural-language product names cannot be produced exactly by a parser.",
			"Only the immutable user request, its validated application context, and the exact candidate-count and byte bounds.",
			"One bounded raw-line inventory of untrusted atomic runtime-outcome candidates or the permitted semantic absence.",
			"DecodeApplicationRequirementInventory parses and binds the inventory before code alone owns the authorization-first candidate queue, local discards, partitioning, duplicate removal, accepted leaves, and exhaustion.")
	case WorkApplicationRequirementCandidateCardinality:
		contract = semanticUncertaintyContract(kind,
			"How many independently testable runtime outcomes does the exact requirement candidate contain?",
			"Semantic outcome multiplicity in natural-language prose cannot be established by byte or syntax validation.",
			"One exact requirement candidate and the registered one-or-multiple outcome vocabulary.",
			"One registered requirement-candidate cardinality relation.",
			"DecodeApplicationRequirementCandidateCardinalityResult validates the relation before code appends the candidate or opens bounded splitting.")
	case WorkApplicationRequirementCandidateKind:
		contract = semanticUncertaintyContractV3(kind,
			"Does the exact requirement candidate contain the code-selected content dimension?",
			"Whether natural-language prose directly states runtime behavior or contains non-runtime constraints cannot be established by byte or syntax validation.",
			"One exact requirement candidate, one code-selected registered content dimension, and the registered present-or-absent vocabulary.",
			"One candidate-and-dimension-bound content-presence relation.",
			"DecodeApplicationRequirementCandidateContentPresenceResult validates the exact presence receipt; code alone combines exactly the two independently bound runtime-content and non-runtime-content receipts into the candidate kind.")
	case WorkApplicationRequirementCandidateAuthorization:
		contract = semanticUncertaintyContractV7(kind,
			"Is the complete semantic content of this exact unclassified candidate entailed by the immutable current request?",
			"Semantic entailment, including direct-imperative grammatical normalization, purpose-name core meaning, and construction-only constraints, cannot be established by syntax or byte comparison.",
			"The immutable request, established application facts, and one exact unclassified inventory or partition candidate.",
			"One request-and-candidate-bound entailed-or-not-entailed relation.",
			"DecodeApplicationRequirementCandidateAuthorizationResult binds the relation to request authority; code alone discards a candidate containing any unstated semantic detail immediately or permits only that candidate to enter downstream classification.")
	case WorkApplicationRequirementCandidateOutcomeRelation:
		contract = semanticUncertaintyContract(kind,
			"Do the exact current and accepted one-outcome runtime requirements express the same independently testable outcome or distinct outcomes?",
			"Semantic outcome equivalence between byte-different natural-language statements cannot be established by syntax or byte comparison.",
			"One exact current candidate, one exact accepted requirement, and the registered same-or-distinct outcome vocabulary.",
			"One registered pair-bound runtime-outcome relation.",
			"DecodeApplicationRequirementCandidateOutcomeRelationResult validates both statement hashes before code discards the new duplicate or continues candidate validation; accepted state is never reopened.")
	case WorkApplicationRequirementCandidateResultRelation:
		contract = semanticUncertaintyContractV3(kind,
			"Does the exact one-outcome runtime requirement contain the code-selected result dimension?",
			"Whether natural-language outcome semantics assert a derived value, and whether that value has an independently computable determining relation, cannot be established by syntax or byte validation.",
			"One exact requirement candidate, its TASK_LOCAL_RUNTIME_OUTCOME and ONE_RUNTIME_OUTCOME receipts, one code-selected result dimension, and for determining-relation presence only the positive derived-value receipt.",
			"One candidate-dimension-bound present-or-absent relation.",
			"DecodeApplicationRequirementCandidateResultPresenceResult validates each binary receipt; code alone combines them into NO_DERIVED_RESULT, EXPLICIT_DERIVED_RESULT_RELATION, or MISSING_DERIVED_RESULT_RELATION before retaining or grounding the candidate.")
	case WorkApplicationRequirementCandidateResultRelationGrounding:
		contract = semanticUncertaintyContractV2(kind,
			"Do the immutable request and established application facts entail exactly one determining relation for the exact candidate outcome whose derived-result relation is missing?",
			"A candidate-only missing-relation receipt cannot establish whether natural-language request and verified context authority uniquely supply the omitted semantic rule.",
			"The immutable request, validated application context, exact current candidate, and its code-bound MISSING_DERIVED_RESULT_RELATION receipt.",
			"One registered request-context-and-candidate-bound entailment relation.",
			"DecodeApplicationRequirementCandidateResultRelationGroundingResult validates the receipt before code discards only that underdetermined candidate or opens one exact one-leaf correction.")
	case WorkApplicationRequirementCandidateResultRelationCorrection:
		contract = semanticUncertaintyContract(kind,
			"What complete replacement corrects the exact one-outcome requirement whose derived-result relation is underdetermined?",
			"The exact determining relation must be expressed from immutable natural-language authority and cannot be supplied by structural validation.",
			"The immutable request, validated application context, exact current candidate, code-bound MISSING_DERIVED_RESULT_RELATION defect, and positive request-context-and-candidate-bound grounding receipt.",
			"One complete byte-different one-outcome runtime requirement with an explicit determining relation.",
			"DecodeApplicationRequirementCandidateResultRelationCorrectionLeaf validates the replacement before code reruns ordinary kind, cardinality, and result-relation validation.")
	case WorkApplicationRequirementCandidatePartition:
		contract = semanticUncertaintyContractV3(kind,
			"What complete source-ordered proper refinement is contained in this exact compound candidate?",
			"Lossless semantic partitioning of mixed or multi-outcome natural-language prose cannot be performed by syntax or byte operations.",
			"One exact compound candidate and exactly one code-bound mixed-kind or multi-outcome receipt.",
			"One bounded raw-line proper refinement containing every child meaning exactly once and no classification or control metadata.",
			"DecodeApplicationRequirementCandidatePartition binds every child to its parent receipt before code replaces that parent with a code-owned child queue.")
	case WorkApplicationProjectStackConstraint:
		contract = semanticUncertaintyContractV2(kind,
			"Which registered technical format and packaging shape, if any, is explicitly established by the immutable software request?",
			"Natural-language technical constraints cannot be mapped exactly to a registered candidate by syntax alone.",
			"The immutable user request and code-enumerated technical-format and packaging-shape candidates.",
			"One opaque technical-format candidate ID.",
			"DecodeApplicationProjectStackConstraintDecision validates the ID before code selects the registered stack adapter.")
	case WorkApplicationClassify:
		contract = semanticUncertaintyContract(kind,
			"Which registered observable delivery surface does the software request require?",
			"Variable human phrasing cannot be mapped exactly to a delivery surface by lexical rules.",
			"The exact immutable software request and the closed delivery-surface vocabulary.",
			"One registered application classification.",
			"DecodeApplicationClassification validates the classification before code selects the registered technical pipeline.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
