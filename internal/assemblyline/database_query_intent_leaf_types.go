package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
)

const (
	WorkDatabaseQueryFromRelation         WorkKind = "database_query_from_relation"
	WorkDatabaseQueryShape                WorkKind = "database_query_shape"
	WorkDatabaseQueryProjectionCoverage   WorkKind = "database_query_projection_coverage"
	WorkDatabaseQueryProjectionAggregate  WorkKind = "database_query_projection_aggregate"
	WorkDatabaseQueryProjectionField      WorkKind = "database_query_projection_field"
	WorkDatabaseQueryProjectionTimeBucket WorkKind = "database_query_projection_time_bucket"
	WorkDatabaseQueryFilterCoverage       WorkKind = "database_query_filter_coverage"
	WorkDatabaseQueryFilterField          WorkKind = "database_query_filter_field"
	WorkDatabaseQueryFilterOperator       WorkKind = "database_query_filter_operator"
	WorkDatabaseQueryFilterValueCoverage  WorkKind = "database_query_filter_value_coverage"
	WorkDatabaseQueryFilterValue          WorkKind = "database_query_filter_value"
	WorkDatabaseQueryWindowCoverage       WorkKind = "database_query_window_coverage"
	WorkDatabaseQueryWindowField          WorkKind = "database_query_window_field"
	WorkDatabaseQueryWindowUnit           WorkKind = "database_query_window_unit"
	WorkDatabaseQueryWindowAmount         WorkKind = "database_query_window_amount"
	WorkDatabaseQueryExistenceCoverage    WorkKind = "database_query_existence_coverage"
	WorkDatabaseQueryExistenceRelation    WorkKind = "database_query_existence_relation"
	WorkDatabaseQueryExistenceNegated     WorkKind = "database_query_existence_negated"
	WorkDatabaseQueryHavingCoverage       WorkKind = "database_query_having_coverage"
	WorkDatabaseQueryHavingAggregate      WorkKind = "database_query_having_aggregate"
	WorkDatabaseQueryHavingField          WorkKind = "database_query_having_field"
	WorkDatabaseQueryHavingOperator       WorkKind = "database_query_having_operator"
	WorkDatabaseQueryHavingValue          WorkKind = "database_query_having_value"
	WorkDatabaseQueryOrderCoverage        WorkKind = "database_query_order_coverage"
	WorkDatabaseQueryOrderProjection      WorkKind = "database_query_order_projection"
	WorkDatabaseQueryOrderDirection       WorkKind = "database_query_order_direction"

	DatabaseQueryItemRemains      = "ITEM_REMAINS"
	DatabaseQueryNoUncoveredItem  = "NO_UNCOVERED_ITEM"
	DatabaseQueryValueRemains     = "VALUE_REMAINS"
	DatabaseQueryNoUncoveredValue = "NO_UNCOVERED_VALUE"
	maxDatabaseQueryLeafBytes     = 2 * 1024
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
	Aggregate datasource.AggregateOperation `json:"aggregate"`
	FieldID   string                        `json:"field_id"`
}

// DatabaseQueryFilterLeafInput is used for a top-level filter when
// ScopeRelationID is empty and for one current existence predicate otherwise.
type DatabaseQueryFilterLeafInput struct {
	State           DatabaseQueryIntentLeafState     `json:"state"`
	ScopeRelationID string                           `json:"scope_relation_id"`
	AcceptedFilters []datasource.RelationalPredicate `json:"accepted_filters"`
	FieldID         string                           `json:"field_id"`
	Operator        datasource.FilterOperator        `json:"operator"`
	AcceptedValues  []datasource.IntentLiteral       `json:"accepted_values"`
}

type DatabaseQueryWindowLeafInput struct {
	State   DatabaseQueryIntentLeafState `json:"state"`
	FieldID string                       `json:"field_id"`
	Unit    datasource.WindowUnit        `json:"unit"`
}

type DatabaseQueryExistenceLeafInput struct {
	State      DatabaseQueryIntentLeafState     `json:"state"`
	RelationID string                           `json:"relation_id"`
	Negated    *bool                            `json:"negated"`
	Filters    []datasource.RelationalPredicate `json:"filters"`
}

type DatabaseQueryHavingLeafInput struct {
	State     DatabaseQueryIntentLeafState  `json:"state"`
	Aggregate datasource.AggregateOperation `json:"aggregate"`
	FieldID   string                        `json:"field_id"`
	Operator  datasource.FilterOperator     `json:"operator"`
}

type DatabaseQueryOrderLeafInput struct {
	State      DatabaseQueryIntentLeafState `json:"state"`
	Projection *int                         `json:"projection"`
}

func NewDatabaseQueryFromRelationJob(input DatabaseQueryIntentLeafState) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryFromRelation, input, input.validate)
}

func NewDatabaseQueryShapeJob(input DatabaseQueryIntentLeafState) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryShape, input, input.validate)
}

func NewDatabaseQueryProjectionCoverageJob(input DatabaseQueryIntentLeafState) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryProjectionCoverage, input, input.validateReady)
}

func NewDatabaseQueryProjectionAggregateJob(input DatabaseQueryProjectionLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryProjectionAggregate, input, input.validate)
}

func NewDatabaseQueryProjectionFieldJob(input DatabaseQueryProjectionLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryProjectionField, input, input.validateForField)
}

func NewDatabaseQueryProjectionTimeBucketJob(input DatabaseQueryProjectionLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryProjectionTimeBucket, input, input.validateForTimeBucket)
}

func NewDatabaseQueryFilterCoverageJob(input DatabaseQueryFilterLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryFilterCoverage, input, input.validate)
}

func NewDatabaseQueryFilterFieldJob(input DatabaseQueryFilterLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryFilterField, input, input.validate)
}

func NewDatabaseQueryFilterOperatorJob(input DatabaseQueryFilterLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryFilterOperator, input, input.validateField)
}

func NewDatabaseQueryFilterValueCoverageJob(input DatabaseQueryFilterLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryFilterValueCoverage, input, input.validateOperator)
}

func NewDatabaseQueryFilterValueJob(input DatabaseQueryFilterLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryFilterValue, input, input.validateOperator)
}

func NewDatabaseQueryWindowCoverageJob(input DatabaseQueryIntentLeafState) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryWindowCoverage, input, input.validateReady)
}

func NewDatabaseQueryWindowFieldJob(input DatabaseQueryWindowLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryWindowField, input, input.validate)
}

func NewDatabaseQueryWindowUnitJob(input DatabaseQueryWindowLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryWindowUnit, input, input.validateField)
}

func NewDatabaseQueryWindowAmountJob(input DatabaseQueryWindowLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryWindowAmount, input, input.validateUnit)
}

func NewDatabaseQueryExistenceCoverageJob(input DatabaseQueryIntentLeafState) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryExistenceCoverage, input, input.validateReady)
}

func NewDatabaseQueryExistenceRelationJob(input DatabaseQueryExistenceLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryExistenceRelation, input, input.validate)
}

func NewDatabaseQueryExistenceNegatedJob(input DatabaseQueryExistenceLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryExistenceNegated, input, input.validateRelation)
}

func NewDatabaseQueryHavingCoverageJob(input DatabaseQueryIntentLeafState) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryHavingCoverage, input, input.validateReady)
}

func NewDatabaseQueryHavingAggregateJob(input DatabaseQueryHavingLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryHavingAggregate, input, input.validate)
}

func NewDatabaseQueryHavingFieldJob(input DatabaseQueryHavingLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryHavingField, input, input.validateAggregate)
}

func NewDatabaseQueryHavingOperatorJob(input DatabaseQueryHavingLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryHavingOperator, input, input.validateField)
}

func NewDatabaseQueryHavingValueJob(input DatabaseQueryHavingLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryHavingValue, input, input.validateOperator)
}

func NewDatabaseQueryOrderCoverageJob(input DatabaseQueryIntentLeafState) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryOrderCoverage, input, input.validateReady)
}

func NewDatabaseQueryOrderProjectionJob(input DatabaseQueryOrderLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryOrderProjection, input, input.validate)
}

func NewDatabaseQueryOrderDirectionJob(input DatabaseQueryOrderLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryOrderDirection, input, input.validateProjection)
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
