package assemblyline

// ExactSemanticLeafCalls is the only valid provider-call count for one raw
// semantic leaf. A code-owned candidate queue may resolve several independent
// leaves, but each leaf is a distinct station invocation and has no retry
// multiplier.
const ExactSemanticLeafCalls = 1
