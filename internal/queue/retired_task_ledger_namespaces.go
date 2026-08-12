package queue

// These prefixes reserve identifiers written by the removed model-driven
// cognition runtime. They grant no runtime behavior; the general Task Ledger
// boundary rejects attempts to manufacture records in retired namespaces.
const (
	retiredCognitionObligationNodePrefix = "cognition_obligation_"
	retiredCognitionObligationEdgePrefix = "cognition_edge_"
	retiredCognitionEntryPrefix          = "cognition_entry_"
)
