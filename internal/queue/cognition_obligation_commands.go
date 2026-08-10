package queue

type CognitionObligationCommandKind string

const (
	CognitionObligationInitial     CognitionObligationCommandKind = "initial"
	CognitionObligationFail        CognitionObligationCommandKind = "fail"
	CognitionObligationSatisfy     CognitionObligationCommandKind = "satisfy"
	CognitionObligationMaterialize CognitionObligationCommandKind = "materialize"
	CognitionObligationPlanRevise  CognitionObligationCommandKind = "plan_revision"
)
