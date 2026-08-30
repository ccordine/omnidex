package assemblyline

type applicationStateFieldPurposeInventoryProjection struct {
	Authority ApplicationServiceStateInterfaceInput `json:"authority"`
}

type applicationStateFieldKindProjection struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	FocusedPurpose string                                `json:"focused_purpose"`
}

type applicationRecordFieldPurposeInventoryProjection struct {
	Authority     ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentPurpose string                                `json:"parent_purpose"`
}

type applicationRecordFieldKindProjection struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentPurpose  string                                `json:"parent_purpose"`
	FocusedPurpose string                                `json:"focused_purpose"`
}
