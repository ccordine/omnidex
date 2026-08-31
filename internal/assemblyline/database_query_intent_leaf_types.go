package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
)

const (
	WorkDatabaseQueryFromRelation         WorkKind = "database_query_from_relation"
	WorkDatabaseQueryShape                WorkKind = "database_query_shape"
	WorkDatabaseQueryProjectionAggregate  WorkKind = "database_query_projection_aggregate"
	WorkDatabaseQueryProjectionField      WorkKind = "database_query_projection_field"
	WorkDatabaseQueryProjectionTimeBucket WorkKind = "database_query_projection_time_bucket"
	WorkDatabaseQueryFilterField          WorkKind = "database_query_filter_field"
	WorkDatabaseQueryFilterOperator       WorkKind = "database_query_filter_operator"
	WorkDatabaseQueryFilterValue          WorkKind = "database_query_filter_value"
	WorkDatabaseQueryWindowField          WorkKind = "database_query_window_field"
	WorkDatabaseQueryWindowUnit           WorkKind = "database_query_window_unit"
	WorkDatabaseQueryWindowAmount         WorkKind = "database_query_window_amount"
	WorkDatabaseQueryExistenceRelation    WorkKind = "database_query_existence_relation"
	WorkDatabaseQueryExistenceNegated     WorkKind = "database_query_existence_negated"
	WorkDatabaseQueryHavingAggregate      WorkKind = "database_query_having_aggregate"
	WorkDatabaseQueryHavingField          WorkKind = "database_query_having_field"
	WorkDatabaseQueryHavingOperator       WorkKind = "database_query_having_operator"
	WorkDatabaseQueryHavingValue          WorkKind = "database_query_having_value"
	WorkDatabaseQueryOrderProjection      WorkKind = "database_query_order_projection"
	WorkDatabaseQueryOrderDirection       WorkKind = "database_query_order_direction"

	maxDatabaseQueryLeafBytes = 2 * 1024
)

// DatabaseQueryIntentLeafState is code-owned partial intent state. Every
// slice contains only complete leaves already accepted by code. A model sees
// this state only to answer the next named semantic question.
type DatabaseQueryIntentLeafState struct {
	Authority       DatabaseQueryIntentInput          `json:"authority"`
	FromRelationID  string                            `json:"from_relation_id"`
	Shape           datasource.ResultShape            `json:"shape"`
	Projections     []datasource.RelationalProjection `json:"projections"`
	Filters         []datasource.RelationalPredicate  `json:"filters"`
	TemporalWindows []DatabaseTemporalWindowDecision  `json:"temporal_windows"`
	Exists          []datasource.ExistencePredicate   `json:"exists"`
	Having          []datasource.AggregatePredicate   `json:"having"`
	OrderBy         []datasource.OrderTerm            `json:"order_by"`
}

type DatabaseQueryProjectionLeafInput struct {
	State     DatabaseQueryIntentLeafState  `json:"state"`
	Purpose   string                        `json:"purpose"`
	Aggregate datasource.AggregateOperation `json:"aggregate"`
	FieldID   string                        `json:"field_id"`
}

// DatabaseQueryFilterLeafInput is used for a top-level filter when
// ScopeRelationID is empty and for one current existence predicate otherwise.
type DatabaseQueryFilterLeafInput struct {
	State           DatabaseQueryIntentLeafState     `json:"state"`
	ScopeRelationID string                           `json:"scope_relation_id"`
	Purpose         string                           `json:"purpose"`
	ParentPurpose   string                           `json:"parent_purpose"`
	AcceptedFilters []datasource.RelationalPredicate `json:"accepted_filters"`
	FieldID         string                           `json:"field_id"`
	Operator        datasource.FilterOperator        `json:"operator"`
	AcceptedValues  []datasource.IntentLiteral       `json:"accepted_values"`
}

type DatabaseQueryWindowLeafInput struct {
	State   DatabaseQueryIntentLeafState `json:"state"`
	Purpose string                       `json:"purpose"`
	FieldID string                       `json:"field_id"`
	Unit    datasource.WindowUnit        `json:"unit"`
}

type DatabaseQueryExistenceLeafInput struct {
	State      DatabaseQueryIntentLeafState     `json:"state"`
	Purpose    string                           `json:"purpose"`
	RelationID string                           `json:"relation_id"`
	Negated    *bool                            `json:"negated"`
	Filters    []datasource.RelationalPredicate `json:"filters"`
}

type DatabaseQueryHavingLeafInput struct {
	State     DatabaseQueryIntentLeafState  `json:"state"`
	Purpose   string                        `json:"purpose"`
	Aggregate datasource.AggregateOperation `json:"aggregate"`
	FieldID   string                        `json:"field_id"`
	Operator  datasource.FilterOperator     `json:"operator"`
}

type DatabaseQueryOrderLeafInput struct {
	State      DatabaseQueryIntentLeafState `json:"state"`
	Purpose    string                       `json:"purpose"`
	Projection *int                         `json:"projection"`
}

func NewDatabaseQueryFromRelationJob(input DatabaseQueryIntentLeafState) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryFromRelation, input)
}

func NewDatabaseQueryShapeJob(input DatabaseQueryIntentLeafState) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryShape, input)
}

func NewDatabaseQueryProjectionAggregateJob(input DatabaseQueryProjectionLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryProjectionAggregate, input)
}

func NewDatabaseQueryProjectionFieldJob(input DatabaseQueryProjectionLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryProjectionField, input)
}

func NewDatabaseQueryProjectionTimeBucketJob(input DatabaseQueryProjectionLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryProjectionTimeBucket, input)
}

func NewDatabaseQueryFilterFieldJob(input DatabaseQueryFilterLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryFilterField, input)
}

func NewDatabaseQueryFilterOperatorJob(input DatabaseQueryFilterLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryFilterOperator, input)
}

func NewDatabaseQueryFilterValueJob(input DatabaseQueryFilterLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryFilterValue, input)
}

func NewDatabaseQueryWindowFieldJob(input DatabaseQueryWindowLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryWindowField, input)
}

func NewDatabaseQueryWindowUnitJob(input DatabaseQueryWindowLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryWindowUnit, input)
}

func NewDatabaseQueryWindowAmountJob(input DatabaseQueryWindowLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryWindowAmount, input)
}

func NewDatabaseQueryExistenceRelationJob(input DatabaseQueryExistenceLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryExistenceRelation, input)
}

func NewDatabaseQueryExistenceNegatedJob(input DatabaseQueryExistenceLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryExistenceNegated, input)
}

func NewDatabaseQueryHavingAggregateJob(input DatabaseQueryHavingLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryHavingAggregate, input)
}

func NewDatabaseQueryHavingFieldJob(input DatabaseQueryHavingLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryHavingField, input)
}

func NewDatabaseQueryHavingOperatorJob(input DatabaseQueryHavingLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryHavingOperator, input)
}

func NewDatabaseQueryHavingValueJob(input DatabaseQueryHavingLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryHavingValue, input)
}

func NewDatabaseQueryOrderProjectionJob(input DatabaseQueryOrderLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryOrderProjection, input)
}

func NewDatabaseQueryOrderDirectionJob(input DatabaseQueryOrderLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryOrderDirection, input)
}

func NewDatabaseQueryIntentLeafState(input DatabaseQueryIntentInput) DatabaseQueryIntentLeafState {
	return DatabaseQueryIntentLeafState{
		Authority: input, Projections: []datasource.RelationalProjection{},
		Filters:         []datasource.RelationalPredicate{},
		TemporalWindows: []DatabaseTemporalWindowDecision{},
		Exists:          []datasource.ExistencePredicate{}, Having: []datasource.AggregatePredicate{},
		OrderBy: []datasource.OrderTerm{},
	}
}

func (state DatabaseQueryIntentLeafState) validate() error {
	if err := state.Authority.validate(); err != nil {
		return err
	}
	if state.FromRelationID != "" && !databaseQueryRelationExists(state, state.FromRelationID) {
		return fmt.Errorf("database query from relation ID %q was not projected", state.FromRelationID)
	}
	if state.Shape != "" && !validDatabaseQueryShape(state.Shape) {
		return fmt.Errorf("database query shape %q is not registered", state.Shape)
	}
	if state.Projections == nil || state.Filters == nil || state.TemporalWindows == nil ||
		state.Exists == nil || state.Having == nil || state.OrderBy == nil {
		return fmt.Errorf("database query partial collections must be explicit")
	}
	if len(state.Projections) > datasource.MaxIntentProjections ||
		len(state.Filters) > datasource.MaxIntentFilters ||
		len(state.TemporalWindows) > datasource.MaxIntentFilters ||
		len(state.Exists) > datasource.MaxIntentExistenceChecks ||
		len(state.Having) > datasource.MaxIntentGroups ||
		len(state.OrderBy) > datasource.MaxIntentOrderTerms {
		return fmt.Errorf("database query partial state exceeds a collection bound")
	}
	if len(state.Projections)+len(state.Filters)+len(state.TemporalWindows)+
		len(state.Exists)+len(state.Having)+len(state.OrderBy) > 0 {
		if state.FromRelationID == "" || state.Shape == "" {
			return fmt.Errorf("database query partial results require accepted from-relation and shape authority")
		}
		if err := validateDatabaseQueryPartialState(state); err != nil {
			return err
		}
	}
	return nil
}
