package assemblyline

import (
	"fmt"
	"strings"
)

const (
	SkillProcedureSchemaV1 = "omnidex.skill-procedure.v1"
	maxSkillProcedureBytes = 800
	maxSkillProcedureNeed  = 1024
	maxSkillLocalContext   = 512
)

type SkillBoundary string

const SkillBoundaryTypeScriptReactView SkillBoundary = "typescript_react_view"

type SkillProcedureInput struct {
	LocalContext string        `json:"local_context"`
	Need         string        `json:"need"`
	Boundary     SkillBoundary `json:"boundary"`
}

type SkillProcedureDecision struct {
	Schema    string `json:"schema"`
	Procedure string `json:"procedure"`
}

func (input SkillProcedureInput) validate() error {
	if input.LocalContext == "" || input.LocalContext != strings.TrimSpace(input.LocalContext) {
		return fmt.Errorf("skill procedure requires one trimmed local context")
	}
	if len(input.LocalContext) > maxSkillLocalContext {
		return fmt.Errorf("skill procedure local context exceeds %d bytes", maxSkillLocalContext)
	}
	if input.Need == "" || input.Need != strings.TrimSpace(input.Need) {
		return fmt.Errorf("skill procedure requires one trimmed local need")
	}
	if len(input.Need) > maxSkillProcedureNeed {
		return fmt.Errorf("skill procedure need exceeds %d bytes", maxSkillProcedureNeed)
	}
	if input.Boundary != SkillBoundaryTypeScriptReactView {
		return fmt.Errorf("skill procedure boundary %q is unsupported", input.Boundary)
	}
	return nil
}

func (decision SkillProcedureDecision) ValidateFor(input SkillProcedureInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != SkillProcedureSchemaV1 {
		return fmt.Errorf("skill procedure schema must be %q", SkillProcedureSchemaV1)
	}
	if decision.Procedure == "" || decision.Procedure != strings.TrimSpace(decision.Procedure) {
		return fmt.Errorf("skill procedure must be one non-empty trimmed instruction")
	}
	if len(decision.Procedure) > maxSkillProcedureBytes {
		return fmt.Errorf("skill procedure exceeds %d bytes", maxSkillProcedureBytes)
	}
	return nil
}

func BuildSkillProcedurePrompt(input SkillProcedureInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Write one reusable procedure for implementing exactly one local behavior inside the supplied technical boundary.",
		"Describe concrete implementation mechanics in imperative form. Do not write source code, choose architecture, add features, mention documents or orchestration, or discuss the wider application. The procedure must stand alone and stay within the stated need.",
		"TECHNICAL_BOUNDARY: " + string(input.Boundary),
		"LOCAL_CONTEXT:\n" + input.LocalContext,
		"LOCAL_NEED:\n" + input.Need,
	}, "\n\n"), nil
}

func SkillProcedureResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "procedure"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": SkillProcedureSchemaV1},
			"procedure": map[string]any{
				"type": "string", "minLength": 1, "maxLength": maxSkillProcedureBytes,
			},
		},
	)
}
