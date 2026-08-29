package assemblyline

type applicationServiceStateRecordSemanticProjection struct {
	Purpose string                           `json:"purpose"`
	Kind    ApplicationServiceStateFieldKind `json:"kind"`
}

type applicationServiceStateFieldSemanticProjection struct {
	Purpose      string                                            `json:"purpose"`
	Kind         ApplicationServiceStateFieldKind                  `json:"kind"`
	RecordFields []applicationServiceStateRecordSemanticProjection `json:"record_fields"`
}

type applicationStateFieldLeafProjection struct {
	Authority      ApplicationServiceStateInterfaceInput            `json:"authority"`
	AcceptedFields []applicationServiceStateFieldSemanticProjection `json:"accepted_fields"`
}

type applicationStateFieldKindProjection struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	FocusedPurpose string                                `json:"focused_purpose"`
}

type applicationRecordFieldLeafProjection struct {
	Authority            ApplicationServiceStateInterfaceInput             `json:"authority"`
	ParentPurpose        string                                            `json:"parent_purpose"`
	AcceptedRecordFields []applicationServiceStateRecordSemanticProjection `json:"accepted_record_fields"`
}

type applicationRecordFieldKindProjection struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentPurpose  string                                `json:"parent_purpose"`
	FocusedPurpose string                                `json:"focused_purpose"`
}

func projectApplicationStateFields(
	fields []ApplicationServiceStateField,
) []applicationServiceStateFieldSemanticProjection {
	projected := make([]applicationServiceStateFieldSemanticProjection, len(fields))
	for index, field := range fields {
		projected[index] = applicationServiceStateFieldSemanticProjection{
			Purpose:      field.Purpose,
			Kind:         field.Kind,
			RecordFields: projectApplicationRecordFields(field.RecordFields),
		}
	}
	return projected
}

func projectApplicationRecordFields(
	fields []ApplicationServiceStateRecordField,
) []applicationServiceStateRecordSemanticProjection {
	projected := make([]applicationServiceStateRecordSemanticProjection, len(fields))
	for index, field := range fields {
		projected[index] = applicationServiceStateRecordSemanticProjection{
			Purpose: field.Purpose, Kind: field.Kind,
		}
	}
	return projected
}
