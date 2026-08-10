package labyrinth

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

type extendedBuilder struct {
	config     ExtendedGeneratorConfig
	entityTick int
	entities   []Entity
	predicates []PredicateSchema
	facts      []cognition.Predicate
	records    []PublicRecord
	actions    []ActionDefinition
	schemas    []cognition.ActionSchema
	witness    []WitnessAction
	uses       []EvidenceUse
	rails      []ExtendedInvalidRail
	omissions  []ExtendedOmissionRail
}

func newExtendedBuilder(config ExtendedGeneratorConfig) *extendedBuilder {
	return &extendedBuilder{config: config}
}

func (builder *extendedBuilder) entity(base string, kind EntityKind, public bool) EntityID {
	builder.entityTick++
	digest, _, err := digestJSON(struct {
		Schema       string `json:"schema"`
		Seed         uint64 `json:"seed"`
		Index        int    `json:"index"`
		PrivateLabel string `json:"private_label"`
	}{"labyrinth.extended-entity.v1", builder.config.Seed, builder.entityTick, base})
	if err != nil {
		panic(fmt.Sprintf("hash extended entity identity: %v", err))
	}
	id := EntityID("entity-" + digest[:20])
	builder.entities = append(builder.entities, Entity{ID: id, Kind: kind, Public: public})
	return id
}

func (builder *extendedBuilder) boundEntity(id EntityID, kind EntityKind, public bool) EntityID {
	builder.entities = append(builder.entities, Entity{ID: id, Kind: kind, Public: public})
	return id
}

func (builder *extendedBuilder) predicate(
	name cognition.PredicateName,
	kinds []EntityKind,
	public bool,
) {
	builder.predicates = append(builder.predicates, PredicateSchema{
		Name: name, ArgumentKinds: append([]EntityKind{}, kinds...), Public: public,
	})
}

func (builder *extendedBuilder) fact(name cognition.PredicateName, entities ...EntityID) error {
	args := make([]string, len(entities))
	for index, entity := range entities {
		args[index] = string(entity)
	}
	predicate, err := cognition.NewPredicate(name, args)
	if err != nil {
		return err
	}
	builder.facts = append(builder.facts, predicate)
	return nil
}

func (builder *extendedBuilder) record(id, location EntityID, content string) (EvidenceIdentity, error) {
	record, err := NewPublicRecord(id, location, content)
	if err != nil {
		return EvidenceIdentity{}, err
	}
	builder.records = append(builder.records, record)
	return EvidenceIdentity{ID: string(id), SHA256: record.ContentSHA256}, nil
}

func (builder *extendedBuilder) action(
	kind cognition.ActionKind,
	parameters []cognition.ActionArgumentName,
	evidence cognition.EvidencePolicy,
	preconditions []Condition,
	effects []Effect,
	cost int,
) (cognition.ActionSchema, error) {
	specs := make([]cognition.ActionParameterSpec, len(parameters))
	for index, name := range parameters {
		specs[index] = cognition.ActionParameterSpec{
			Name: name, Required: true, MaxBytes: cognition.MaxActionValueBytes,
		}
	}
	schema, err := cognition.NewActionSchema(
		cognition.ActionSchemaID("labyrinth.action."+string(kind)+".v1"),
		GrammarVersionV1, kind, specs, evidence,
	)
	if err != nil {
		return cognition.ActionSchema{}, err
	}
	builder.schemas = append(builder.schemas, schema)
	builder.actions = append(builder.actions, ActionDefinition{
		Schema: schema, LiteralParameters: []LiteralParameter{},
		Preconditions: append([]Condition{}, preconditions...),
		Effects:       append([]Effect{}, effects...), Cost: cost,
	})
	return schema, nil
}

func (builder *extendedBuilder) witnessAction(
	schema cognition.ActionSchema,
	arguments ...cognition.ActionArgument,
) (WitnessAction, error) {
	request, err := cognition.NewActionRequest(schema.Kind, arguments)
	if err != nil {
		return WitnessAction{}, err
	}
	action := WitnessAction{
		ID:     cognition.ActionID(fmt.Sprintf("witness-%s-%02d", builder.config.Suite, len(builder.witness)+1)),
		Schema: schema.Ref(), Request: request, Cost: builder.actionCost(schema.Kind),
	}
	builder.witness = append(builder.witness, action)
	return action, nil
}

func (builder *extendedBuilder) actionCost(kind cognition.ActionKind) int {
	for _, action := range builder.actions {
		if action.Schema.Kind == kind {
			return action.Cost
		}
	}
	panic("extended witness action lacks definition")
}

func (builder *extendedBuilder) invalidRail(
	id string,
	prefix int,
	code cognition.ActionFailureCode,
	kind cognition.ActionKind,
	arguments ...cognition.ActionArgument,
) error {
	request, err := cognition.NewActionRequest(kind, arguments)
	if err != nil {
		return err
	}
	builder.rails = append(builder.rails, ExtendedInvalidRail{
		ID: id, PrefixActions: prefix, Request: request,
		Outcome: ExtendedRailRejected, FailureCode: code,
	})
	return nil
}

func (builder *extendedBuilder) deadEndRail(
	id string,
	prefix int,
	kind cognition.ActionKind,
	arguments ...cognition.ActionArgument,
) error {
	request, err := cognition.NewActionRequest(kind, arguments)
	if err != nil {
		return err
	}
	builder.rails = append(builder.rails, ExtendedInvalidRail{
		ID: id, PrefixActions: prefix, Request: request,
		Outcome: ExtendedRailIrreversibleDeadEnd,
	})
	return nil
}

func (builder *extendedBuilder) omissionRail(
	id string,
	omitted int,
	failure int,
	code cognition.ActionFailureCode,
) {
	kind := cognition.ActionKind("")
	if omitted >= 0 && omitted < len(builder.witness) {
		kind = builder.witness[omitted].Request.Kind
	}
	builder.omissions = append(builder.omissions, ExtendedOmissionRail{
		ID: id, OmittedAction: omitted, OmittedKind: kind,
		FailureAction: failure, FailureCode: code,
	})
}

func (builder *extendedBuilder) finish(
	goal cognition.GoalExpression,
	goalText string,
	branching int,
	dependencies int,
) (ExtendedCase, error) {
	relevantRecords := len(builder.records)
	if err := builder.materializeDistractors(MinGeneratedWorldSize); err != nil {
		return ExtendedCase{}, err
	}
	catalog, err := cognition.NewActionCatalog(
		cognition.ActionCatalogID("labyrinth.actions.v1"), GrammarVersionV1, builder.schemas,
	)
	if err != nil {
		return ExtendedCase{}, err
	}
	definition, err := NewDefinition(
		catalog, builder.entities, builder.predicates, builder.facts, builder.actions, goal,
	)
	if err != nil {
		return ExtendedCase{}, err
	}
	sort.Slice(builder.records, func(left, right int) bool { return builder.records[left].ID < builder.records[right].ID })
	descriptor := PublicDescriptor{
		Suite: builder.config.Suite, FormatVersion: "extended-symbolic.v1",
		SurfaceVersion: "symbolic.v1", GrammarVersion: builder.config.GrammarVersion,
		Goal: goalText, Records: append([]PublicRecord{}, builder.records...),
		Difficulty: PublicDifficulty{
			WorldSize: len(builder.records), EvidenceArtifacts: max(1, relevantRecords),
			DecisionDepth: len(builder.witness), BranchingFactor: branching,
			DependencyCount: dependencies,
		},
	}
	scenarioDigest, _, err := digestJSON(struct {
		Schema string                  `json:"schema"`
		Config ExtendedGeneratorConfig `json:"config"`
	}{"labyrinth.extended-scenario-id.v1", builder.config})
	if err != nil {
		return ExtendedCase{}, err
	}
	scenario, err := NewScenario(cognition.ScenarioID("scenario-"+scenarioDigest), definition, descriptor)
	if err != nil {
		return ExtendedCase{}, err
	}
	oracle := ExtendedOracle{
		Schema: ExtendedOracleSchemaV1, ScenarioID: scenario.Ref().ID,
		PublicSHA256: scenario.Ref().SHA256, DefinitionSHA256: definition.SHA256(),
		GeneratorVersion: builder.config.GeneratorVersion, GrammarVersion: builder.config.GrammarVersion,
		Suite: builder.config.Suite, Seed: builder.config.Seed,
		TaskArchetype: extendedArchetype(builder.config.Suite),
		Witness:       cloneWitness(builder.witness), EvidenceUses: append([]EvidenceUse{}, builder.uses...),
		InvalidRails:  append([]ExtendedInvalidRail{}, builder.rails...),
		OmissionRails: append([]ExtendedOmissionRail{}, builder.omissions...),
	}
	if err := oracle.seal(); err != nil {
		return ExtendedCase{}, err
	}
	generated := ExtendedCase{execution: scenario, public: scenario.PublicArtifact(), oracle: oracle}
	return generated, generated.Validate()
}

func argument(name cognition.ActionArgumentName, value EntityID) cognition.ActionArgument {
	return cognition.ActionArgument{Name: name, Value: string(value)}
}

func parameter(name cognition.ActionArgumentName) PatternArgument {
	return PatternArgument{Parameter: name}
}

func fixed(entity EntityID) PatternArgument { return PatternArgument{Entity: entity} }

func condition(mode ConditionMode, name cognition.PredicateName, arguments ...PatternArgument) Condition {
	return Condition{Mode: mode, Predicate: PredicatePattern{Name: name, Arguments: arguments}}
}

func effect(mode EffectMode, name cognition.PredicateName, arguments ...PatternArgument) Effect {
	return Effect{Mode: mode, Predicate: PredicatePattern{Name: name, Arguments: arguments}}
}
