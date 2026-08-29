package assemblyline

func applicationSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkApplicationContextNeedCoverage:
		contract = semanticUncertaintyContract(kind,
			"Does one necessary repository-fact question remain uncovered by the retained questions?",
			"Semantic coverage of free-form questions cannot be established by structural validation.",
			"The immutable request, established application context, and retained missing-fact questions.",
			"One registered context-need coverage relation.",
			"DecodeApplicationContextNeedCoverageLeaf validates the relation before code continues or closes the bounded fixed point.")
	case WorkApplicationContextNeedQuestion:
		contract = semanticUncertaintyContract(kind,
			"What single necessary repository-fact question remains uncovered?",
			"The missing semantic fact required to interpret free-form authority cannot be derived from repository structure alone.",
			"The immutable request, established application context, and retained missing-fact questions.",
			"One necessary missing-fact question.",
			"DecodeApplicationContextNeedQuestionLeaf validates the question before code appends it to the bounded retained set.")
	case WorkApplicationProductContext:
		contract = semanticUncertaintyContractV2(kind,
			"What concise product or domain identity is explicitly established by the immutable software request and established facts, excluding its requirements?",
			"Product or domain identity expressed in natural language has no mechanically exact structural representation.",
			"The immutable user request and established application facts needed to identify only the product, domain, audience, setting, or purpose.",
			"One concise product-or-domain identity phrase that excludes software requirements.",
			"DecodeApplicationProductContextLeaf validates the leaf before code binds it as accepted application authority.")
	case WorkApplicationRequirementCoverage:
		contract = semanticUncertaintyContractV2(kind,
			"Does one task-local runtime implementation requirement remain uncovered by the retained requirements?",
			"Semantic equivalence and whether free-form behavior requires task-local application source cannot be computed by byte or syntax comparison.",
			"The immutable request, established facts, and retained requirement statements.",
			"One registered task-local application-requirement coverage relation.",
			"DecodeApplicationRequirementCoverageLeaf validates the relation before code continues or closes the bounded fixed point.")
	case WorkApplicationRequirement:
		contract = semanticUncertaintyContractV3(kind,
			"What is the earliest task-local runtime implementation requirement not covered by the retained requirements?",
			"Faithful extraction of one source-owning runtime obligation from unconstrained human phrasing cannot be performed by a parser.",
			"The immutable request, established facts, accepted requirement statements, excluded non-runtime candidates, and the exact code-established REQUIREMENT_REMAINS coverage receipt bound to that authority.",
			"One task-local runtime implementation-requirement text leaf.",
			"DecodeApplicationRequirementLeaf validates the leaf before code appends it to the bounded retained set.")
	case WorkApplicationRequirementCandidateCardinality:
		contract = semanticUncertaintyContract(kind,
			"How many independently testable runtime outcomes does the exact requirement candidate contain?",
			"Semantic outcome multiplicity in natural-language prose cannot be established by byte or syntax validation.",
			"One exact requirement candidate and the registered one-or-multiple outcome vocabulary.",
			"One registered requirement-candidate cardinality relation.",
			"DecodeApplicationRequirementCandidateCardinalityResult validates the relation before code appends the candidate or opens bounded splitting.")
	case WorkApplicationRequirementCandidateKind:
		contract = semanticUncertaintyContract(kind,
			"Does the exact requirement candidate contain only task-local runtime-outcome content or express a non-runtime constraint?",
			"Whether natural-language prose requires application source is a semantic distinction that byte and syntax validation cannot establish.",
			"One exact requirement candidate and the registered task-local-or-non-runtime vocabulary.",
			"One registered requirement-candidate kind relation.",
			"DecodeApplicationRequirementCandidateKindResult validates the candidate-bound relation before code retains the runtime candidate or records the non-runtime candidate as excluded.")
	case WorkApplicationRequirementCandidateSplit:
		contract = semanticUncertaintyContract(kind,
			"What is the earliest single runtime outcome contained in the exact multi-outcome requirement candidate?",
			"The earliest semantic outcome inside compound natural-language prose cannot be extracted by a parser.",
			"One exact requirement candidate and its code-bound MULTIPLE_RUNTIME_OUTCOMES relation.",
			"One single-outcome requirement text leaf.",
			"DecodeApplicationRequirementCandidateSplitLeaf validates a changed leaf before code repeats the bounded cardinality check.")
	case WorkApplicationRequirementCandidateSplitCorrection:
		contract = semanticUncertaintyContract(kind,
			"What complete replacement corrects the exact byte-identical multi-outcome split candidate?",
			"Selecting the earliest semantic outcome inside the preserved defective prose cannot be performed by byte comparison.",
			"The exact current candidate, its code-bound MULTIPLE_RUNTIME_OUTCOMES relation, and the exact byte-identity defect.",
			"One complete byte-different single-outcome requirement text leaf.",
			"DecodeApplicationRequirementCandidateSplitCorrectionLeaf validates the replacement before code repeats ordinary cardinality validation.")
	case WorkApplicationRequirementCandidateDuplicateReplacement:
		contract = semanticUncertaintyContract(kind,
			"What complete requirement replaces the exact candidate that duplicates one indexed retained value?",
			"Selecting a different earliest uncovered semantic outcome from the immutable request cannot be performed by byte comparison.",
			"The exact generation authority and REQUIREMENT_REMAINS receipt, exact duplicate candidate, its code-validated retained-set identity, and the exact duplicate defect.",
			"One complete byte-different requirement text leaf.",
			"DecodeApplicationRequirementCandidateDuplicateReplacementLeaf validates the replacement before code reruns ordinary candidate classification and cardinality checks.")
	case WorkApplicationProjectStackConstraint:
		contract = semanticUncertaintyContractV2(kind,
			"Which registered technical format and packaging shape, if any, is explicitly established by the immutable software request?",
			"Natural-language technical constraints cannot be mapped exactly to a registered candidate by syntax alone.",
			"The immutable user request and code-enumerated technical-format and packaging-shape candidates.",
			"One opaque technical-format candidate ID.",
			"DecodeApplicationProjectStackConstraintDecision validates the ID before code selects the registered stack adapter.")
	case WorkApplicationServiceContinuedAvailability:
		contract = semanticUncertaintyContract(kind,
			"Does the request explicitly require the completed software to remain available after verification?",
			"Continued-availability intent is a semantic distinction not encoded by request syntax.",
			"The immutable user request and the code-enumerated availability candidates.",
			"One opaque continued-availability candidate ID.",
			"DecodeApplicationServiceContinuedAvailabilityResult validates the ID before code records the service-lifecycle fact.")
	case WorkApplicationServicePersistenceDestination:
		contract = semanticUncertaintyContract(kind,
			"Does the request explicitly identify the build environment as the continued-availability destination?",
			"Destination identity may be implied ambiguously in natural language and cannot be proven by token matching.",
			"The immutable user request and the code-enumerated destination candidates.",
			"One opaque persistence-destination candidate ID.",
			"DecodeApplicationServicePersistenceDestinationResult validates the ID before code records the destination fact.")
	case WorkApplicationServiceStateLifetime:
		contract = semanticUncertaintyContract(kind,
			"What registered state lifetime does the exact service requirement require?",
			"Whether behavior crosses request boundaries is semantic intent not derivable from field shape.",
			"The accepted product context and one exact service requirement.",
			"One registered service-state lifetime value.",
			"DecodeApplicationServiceStateLifetimeResult validates the value before code derives state authority.")
	case WorkApplicationStateFieldCoverage:
		contract = semanticUncertaintyContract(kind,
			"Does one necessary durable root-state value remain uncovered by the retained values?",
			"Minimal semantic state coverage cannot be decided from structural field validation.",
			"The directly related behavior authority and retained root-state purposes.",
			"One registered root-state coverage relation.",
			"DecodeApplicationStateFieldCoverageLeaf validates the relation before code continues or closes the bounded field set.")
	case WorkApplicationStateFieldPurpose:
		contract = semanticUncertaintyContract(kind,
			"What single necessary durable root-state responsibility remains uncovered?",
			"A required state responsibility must be inferred from behavior meaning rather than data syntax.",
			"The directly related behavior authority and retained root-state purposes.",
			"One durable root-state purpose sentence.",
			"DecodeApplicationStateFieldPurposeLeaf validates the purpose before code appends it to the state interface.")
	case WorkApplicationStateFieldKind:
		contract = semanticUncertaintyContract(kind,
			"Which registered data kind fulfills the focused durable root-state purpose?",
			"The purpose text does not mechanically determine its minimally sufficient data kind.",
			"The directly related behavior authority and focused root-state purpose.",
			"One registered root-state data kind.",
			"DecodeApplicationStateFieldKindLeaf validates the kind before code binds it to the focused field.")
	case WorkApplicationRecordFieldCoverage:
		contract = semanticUncertaintyContract(kind,
			"Does one necessary scalar record member remain uncovered by the retained members?",
			"Minimal semantic member coverage cannot be decided from record structure alone.",
			"The behavior authority, focused record-list purpose, and retained member purposes.",
			"One registered record-member coverage relation.",
			"DecodeApplicationRecordFieldCoverageLeaf validates the relation before code continues or closes the bounded member set.")
	case WorkApplicationRecordFieldPurpose:
		contract = semanticUncertaintyContract(kind,
			"What single necessary scalar record-member responsibility remains uncovered?",
			"A required member responsibility must be inferred from record behavior meaning.",
			"The behavior authority, focused record-list purpose, and retained member purposes.",
			"One scalar record-member purpose sentence.",
			"DecodeApplicationRecordFieldPurposeLeaf validates the purpose before code appends it to the record interface.")
	case WorkApplicationRecordFieldKind:
		contract = semanticUncertaintyContract(kind,
			"Which registered scalar data kind fulfills the focused record-member purpose?",
			"The member purpose text does not mechanically determine its minimally sufficient scalar kind.",
			"The behavior authority, parent record purpose, and focused member purpose.",
			"One registered scalar record-member kind.",
			"DecodeApplicationRecordFieldKindLeaf validates the kind before code binds it to the focused member.")
	case WorkApplicationServiceEndpointRequirement:
		contract = semanticUncertaintyContract(kind,
			"Does the exact service requirement need a direct HTTP request-response interaction?",
			"Direct endpoint intent is not mechanically implied by a behavior description.",
			"The accepted product context and one exact service requirement.",
			"One registered endpoint-requirement value.",
			"DecodeApplicationServiceEndpointRequirementResult validates the value before code decides whether endpoint leaves exist.")
	case WorkApplicationServiceEndpointExposure:
		contract = semanticUncertaintyContract(kind,
			"Which registered exposure scope may reach the required endpoint?",
			"Audience reachability intent cannot be inferred exactly from HTTP mechanics.",
			"The accepted endpoint requirement authority and compatible exposure values.",
			"One registered endpoint-exposure value.",
			"DecodeApplicationServiceEndpointExposureResult validates the value before code binds endpoint exposure.")
	case WorkApplicationServiceEndpointMethod:
		contract = semanticUncertaintyContract(kind,
			"Which registered HTTP method matches the exact endpoint requirement?",
			"Several methods are mechanically valid while intended operation semantics remain ambiguous.",
			"The accepted endpoint requirement authority and compatible HTTP methods.",
			"One registered HTTP method.",
			"DecodeApplicationServiceEndpointMethodResult validates the method before code derives compatible media leaves.")
	case WorkApplicationServiceEndpointRouteTemplate:
		contract = semanticUncertaintyContract(kind,
			"What normalized route template semantically names the exact endpoint requirement?",
			"Route naming is a semantic choice not derivable from protocol validation.",
			"The accepted endpoint requirement authority and normalized route grammar.",
			"One normalized HTTP route template.",
			"DecodeApplicationServiceEndpointRouteTemplateResult validates the template before code binds endpoint identity.")
	case WorkApplicationServiceEndpointRequestMedia:
		contract = semanticUncertaintyContract(kind,
			"Which registered request media type is required for the endpoint under its accepted method?",
			"Payload representation intent cannot be inferred exactly from the method or protocol grammar.",
			"The accepted endpoint authority, accepted HTTP method, and compatible request-media candidates.",
			"One registered request-media value.",
			"DecodeApplicationServiceEndpointRequestMediaResult validates the value before code binds request representation.")
	case WorkApplicationServiceEndpointResponseMedia:
		contract = semanticUncertaintyContract(kind,
			"Which registered response media type is produced for the endpoint under its accepted method?",
			"Response representation intent cannot be inferred exactly from the method or protocol grammar.",
			"The accepted endpoint authority, accepted HTTP method, and compatible response-media candidates.",
			"One registered response-media value.",
			"DecodeApplicationServiceEndpointResponseMediaResult validates the value before code binds response representation.")
	case WorkApplicationServiceEndpointSuccessStatus:
		contract = semanticUncertaintyContract(kind,
			"Which successful HTTP status matches the endpoint under its accepted prerequisites?",
			"Multiple statuses are protocol-valid while intended outcome semantics remain ambiguous.",
			"The accepted endpoint authority, accepted method, accepted media values, and compatible status candidates.",
			"One registered successful HTTP status.",
			"DecodeApplicationServiceEndpointSuccessStatusResult validates the status before code binds the endpoint outcome.")
	case WorkApplicationClassify:
		contract = semanticUncertaintyContract(kind,
			"Which registered observable delivery surface does the software request require?",
			"Variable human phrasing cannot be mapped exactly to a delivery surface by lexical rules.",
			"The exact immutable software request and the closed delivery-surface vocabulary.",
			"One registered application classification.",
			"DecodeApplicationClassification validates the classification before code selects the registered technical pipeline.")
	case WorkApplicationTargetTree:
		contract = semanticUncertaintyContract(kind,
			"What complete managed workload basename hierarchy satisfies all accepted goals?",
			"Accepted goals admit multiple structurally valid hierarchies that tree rules cannot rank semantically.",
			"The accepted goals, code-selected technical context, exact tree constraints, current managed hierarchy, and reserved hierarchy.",
			"One complete raw basename hierarchy.",
			"DecodeTargetTreeCandidate parses the hierarchy before code constructs normalized paths and filesystem transitions.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}

func historicalApplicationIntentSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkApplicationProductContext:
		contract = semanticUncertaintyContract(kind,
			"What concise product context is explicitly established by the software request?",
			"Product meaning expressed in natural language has no mechanically exact structural representation.",
			"The immutable user request and established application facts.",
			"One concise product-context text leaf.",
			"DecodeApplicationProductContextLeaf validates the leaf before code binds it as accepted application authority.")
	case WorkApplicationRequirementCoverage:
		contract = semanticUncertaintyContract(kind,
			"Does one explicit software requirement remain uncovered by the retained requirements?",
			"Semantic equivalence between free-form requirements cannot be computed by byte or syntax comparison.",
			"The immutable request, product context, established facts, and retained requirement statements.",
			"One registered application-requirement coverage relation.",
			"DecodeApplicationRequirementCoverageLeaf validates the relation before code continues or closes the bounded fixed point.")
	case WorkApplicationRequirement:
		contract = semanticUncertaintyContract(kind,
			"What is the earliest explicit software requirement not covered by the retained requirements?",
			"Faithful semantic extraction from unconstrained human phrasing cannot be performed by a parser.",
			"The immutable request, product context, established facts, and retained requirement statements.",
			"One explicit software-requirement text leaf.",
			"DecodeApplicationRequirementLeaf validates the leaf before code appends it to the bounded retained set.")
	case WorkApplicationProjectStackConstraint:
		contract = semanticUncertaintyContract(kind,
			"Which registered technical format constraint is explicitly established by accepted application authority?",
			"Natural-language technical constraints cannot be mapped exactly to a registered candidate by syntax alone.",
			"The accepted product context, retained requirements, and code-enumerated technical-format candidates.",
			"One opaque technical-format candidate ID.",
			"DecodeApplicationProjectStackConstraintDecision validates the ID before code selects the registered stack adapter.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}

func rendererV7ApplicationIntentSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	switch kind {
	case WorkApplicationRequirementCoverage:
		return semanticUncertaintyContractV2(kind,
			"Does one task-local runtime implementation requirement remain uncovered by the retained requirements?",
			"Semantic equivalence and whether free-form behavior requires task-local application source cannot be computed by byte or syntax comparison.",
			"The immutable request, established facts, and retained requirement statements.",
			"One registered task-local application-requirement coverage relation.",
			"DecodeApplicationRequirementCoverageLeaf validates the relation before code continues or closes the bounded fixed point."), true
	case WorkApplicationRequirement:
		return semanticUncertaintyContractV2(kind,
			"What is the earliest task-local runtime implementation requirement not covered by the retained requirements?",
			"Faithful extraction of one source-owning runtime obligation from unconstrained human phrasing cannot be performed by a parser.",
			"The immutable request, established facts, and retained requirement statements.",
			"One task-local runtime implementation-requirement text leaf.",
			"DecodeApplicationRequirementLeaf validates the leaf before code appends it to the bounded retained set."), true
	default:
		contract, ok := applicationSemanticUncertaintyContract(kind)
		if !ok || contract.ID != semanticUncertaintyContractIDPrefix+string(kind)+".v2" {
			return SemanticUncertaintyContract{}, false
		}
		return contract, true
	}
}
