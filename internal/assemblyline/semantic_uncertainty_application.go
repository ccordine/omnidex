package assemblyline

func applicationSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkApplicationInputSource:
		contract = semanticUncertaintyContract(kind,
			"Which process input does this one accepted behavior require?",
			"The input channel implied by natural language is not exactly determined by syntax or keyword matching.",
			"Only one accepted local behavior and the registered process input channels.",
			"One opaque choice identifying the required input channel.",
			"DecodeApplicationInputSource binds the channel; code supplies only that channel as a typed function parameter.")
	case WorkApplicationResultValueKind:
		contract = semanticUncertaintyContract(kind,
			"What kind of value does this one accepted behavior produce before presentation?",
			"The value kind implied by a natural-language behavior cannot be inferred exactly by syntax or keyword matching.",
			"Only one accepted local behavior and the registered value kinds.",
			"One opaque choice identifying the value kind.",
			"DecodeApplicationResultValueKind binds the kind; the adapter supplies typed declarations, result formatting, and comparisons.")
	case WorkApplicationProductContext:
		contract = semanticUncertaintyContract(kind,
			"What concise product or domain identity is explicitly established by the immutable software request and established facts, excluding its requirements?",
			"Product or domain identity expressed in natural language has no mechanically exact structural representation.",
			"The immutable user request and established application facts needed to identify only the product, domain, audience, setting, or purpose.",
			"One concise product-or-domain identity phrase that excludes software requirements.",
			"DecodeApplicationProductContextLeaf validates the leaf before code binds it as accepted application authority.")
	case WorkApplicationRequirementInventory:
		contract = semanticUncertaintyContract(kind,
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
		contract = semanticUncertaintyContract(kind,
			"Does the exact requirement candidate contain the code-selected content dimension?",
			"Whether natural-language prose directly states runtime behavior or contains non-runtime constraints cannot be established by byte or syntax validation.",
			"One exact requirement candidate, one code-selected registered content dimension, and the registered present-or-absent vocabulary.",
			"One candidate-and-dimension-bound content-presence relation.",
			"DecodeApplicationRequirementCandidateContentPresenceResult validates the exact presence receipt; code alone combines exactly the two independently bound runtime-content and non-runtime-content receipts into the candidate kind.")
	case WorkApplicationRequirementCandidateAuthorization:
		contract = semanticUncertaintyContract(kind,
			"Is the complete semantic content of this exact unclassified candidate entailed by the immutable current request?",
			"Semantic entailment, including direct-imperative grammatical normalization, purpose-name core meaning, and construction-only constraints, cannot be established by syntax or byte comparison.",
			"The immutable request, established application facts, and one exact unclassified inventory or partition candidate.",
			"One request-and-candidate-bound entailed-or-not-entailed relation.",
			"DecodeApplicationRequirementCandidateAuthorizationResult validates the relation; code discards a not-entailed candidate before classification and continues independent candidates without reopening accepted requirements.")
	case WorkApplicationRequirementCandidateOutcomeRelation:
		contract = semanticUncertaintyContract(kind,
			"Do the exact current and accepted one-outcome runtime requirements express the same independently testable outcome or distinct outcomes?",
			"Semantic outcome equivalence between byte-different natural-language statements cannot be established by syntax or byte comparison.",
			"One exact current candidate, one exact accepted requirement, and the registered same-or-distinct outcome vocabulary.",
			"One registered pair-bound runtime-outcome relation.",
			"DecodeApplicationRequirementCandidateOutcomeRelationResult validates the relation for the exact current and accepted statements before code discards the new duplicate or continues candidate validation; accepted state is never reopened.")
	case WorkApplicationRequirementCandidateResultRelation:
		contract = semanticUncertaintyContract(kind,
			"Does the exact one-outcome runtime requirement contain the code-selected result dimension?",
			"Whether natural-language outcome semantics assert a derived value, and whether that value has an independently computable determining relation, cannot be established by syntax or byte validation.",
			"One exact requirement candidate, its TASK_LOCAL_RUNTIME_OUTCOME and ONE_RUNTIME_OUTCOME receipts, one code-selected result dimension, and for determining-relation presence only the positive derived-value receipt.",
			"One candidate-dimension-bound present-or-absent relation.",
			"DecodeApplicationRequirementCandidateResultPresenceResult validates each binary receipt; code alone combines them into NO_DERIVED_RESULT, EXPLICIT_DERIVED_RESULT_RELATION, or MISSING_DERIVED_RESULT_RELATION before retaining or locally discarding only that candidate.")
	case WorkApplicationRequirementCandidatePartition:
		contract = semanticUncertaintyContract(kind,
			"What complete source-ordered proper refinement is contained in this exact compound candidate?",
			"Lossless semantic partitioning of mixed or multi-outcome natural-language prose cannot be performed by syntax or byte operations.",
			"One exact compound candidate and exactly one code-bound mixed-kind or multi-outcome receipt.",
			"One bounded raw-line proper refinement containing every child meaning exactly once and no classification or control metadata.",
			"DecodeApplicationRequirementCandidatePartition binds every child to its parent receipt before code replaces that parent with a code-owned child queue.")
	case WorkApplicationProjectStackConstraint:
		contract = semanticUncertaintyContract(kind,
			"Which registered technical format and packaging shape, if any, is explicitly established by the immutable software request?",
			"Natural-language technical constraints cannot be mapped exactly to a registered candidate by syntax alone.",
			"The immutable user request and code-enumerated technical-format and packaging-shape candidates.",
			"One opaque technical-format candidate ID.",
			"DecodeApplicationProjectStackConstraintDecision validates the ID before code selects the registered stack adapter.")
	case WorkApplicationClassify:
		contract = semanticUncertaintyContract(kind,
			"Which registered observable delivery surface does the software request require, or does it leave delivery unconstrained or explicitly require an unsupported surface?",
			"Variable human phrasing cannot be mapped exactly to a delivery surface by lexical rules.",
			"The exact immutable software request and the closed delivery-surface vocabulary.",
			"One registered application classification, including an unconstrained observation when no surface is required.",
			"DecodeApplicationClassification validates the classification; ResolveApplicationSurface applies the code-owned unconstrained default or rejects unsupported delivery before stack selection.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
