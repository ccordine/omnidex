package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/roleplay"
)

func bindObjectiveModelInstruction(
	authority turnAuthority,
	provenance assemblyline.ArtifactIdentityProvenance,
) (turnAuthority, error) {
	source, err := objectiveModelBindingSource(authority)
	if err != nil {
		return authority, err
	}
	redacted, identities, err := assemblyline.RedactArtifactIdentities(source, provenance)
	if err != nil {
		return authority, fmt.Errorf("redact objective instruction artifact identities: %w", err)
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"objective model instruction projection", provenance, redacted,
	); err != nil {
		return authority, err
	}
	if _, err := assemblyline.RestoreArtifactIdentities(redacted, identities); err != nil {
		return authority, fmt.Errorf("verify objective instruction artifact bindings: %w", err)
	}

	modelInstruction := redacted
	if authority.ChannelMode == model.ChannelModeRoleplay &&
		authority.RoleplayInputKind != roleplay.SimulationTurnExternalCommand {
		modelInstruction, err = roleplayModelVisibleInstruction(
			authority.RoleplayInputKind, redacted,
		)
		if err != nil {
			return authority, fmt.Errorf("project roleplay model instruction: %w", err)
		}
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"objective model instruction", provenance, modelInstruction,
	); err != nil {
		return authority, err
	}
	authority.ModelInstruction = modelInstruction
	authority.ModelRedactedInstruction = redacted
	authority.ModelArtifactIdentities = append(
		[]assemblyline.ArtifactIdentity(nil), identities...,
	)
	authority.ModelArtifactPaths = append([]string{}, provenance.Paths()...)
	return authority, nil
}

func objectiveModelBindingSource(authority turnAuthority) (string, error) {
	switch authority.ChannelMode {
	case model.ChannelModeAssistant:
		return authority.Instruction, nil
	case model.ChannelModeRoleplay:
		if authority.RoleplayInputKind != roleplay.SimulationTurnExternalCommand {
			return authority.Instruction, nil
		}
		command, matched, err := roleplay.ParseResearchCommand(authority.Instruction)
		if err != nil {
			return "", fmt.Errorf("project roleplay research instruction: %w", err)
		}
		if !matched {
			return "", fmt.Errorf("roleplay external command lacks exact research authority")
		}
		return command.Question, nil
	default:
		return "", fmt.Errorf(
			"objective model instruction has unsupported channel mode %q",
			authority.ChannelMode,
		)
	}
}

func objectiveModelProvenance(
	authority turnAuthority,
) (assemblyline.ArtifactIdentityProvenance, error) {
	provenance, err := modelcontext.NewArtifactIdentityProvenance(authority.ModelArtifactPaths)
	if err != nil {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"rebuild objective artifact provenance: %w", err,
		)
	}
	return provenance, nil
}

func validateObjectiveModelInput(
	authority turnAuthority,
	label string,
	values ...string,
) error {
	provenance, err := objectiveModelProvenance(authority)
	if err != nil {
		return err
	}
	return assemblyline.ValidatePathFreeModelContextWithProvenance(
		label, provenance, values...,
	)
}

func restoreObjectiveModelText(
	authority turnAuthority,
	label string,
	value string,
) (string, error) {
	if err := validateObjectiveModelInput(authority, label, value); err != nil {
		return "", err
	}
	restored, err := assemblyline.RestoreArtifactIdentities(
		value, authority.ModelArtifactIdentities,
	)
	if err != nil {
		return "", fmt.Errorf("%s artifact restoration: %w", label, err)
	}
	return restored, nil
}

func restoreObjectiveCodeRenderedArtifact(
	authority turnAuthority,
	label string,
	value string,
) (string, error) {
	restored, err := assemblyline.RestoreArtifactIdentities(
		value, authority.ModelArtifactIdentities,
	)
	if err != nil {
		return "", fmt.Errorf("%s artifact restoration: %w", label, err)
	}
	return restored, nil
}

func restoreObjectiveModelTexts(
	authority turnAuthority,
	label string,
	values []string,
) ([]string, error) {
	restored := make([]string, len(values))
	for index, value := range values {
		current, err := restoreObjectiveModelText(
			authority, fmt.Sprintf("%s %d", label, index), value,
		)
		if err != nil {
			return nil, err
		}
		restored[index] = current
	}
	return restored, nil
}

func restoreObjectiveOptionalModelText(
	authority turnAuthority,
	label string,
	value *string,
) (*string, error) {
	if value == nil {
		return nil, nil
	}
	restored, err := restoreObjectiveModelText(authority, label, *value)
	if err != nil {
		return nil, err
	}
	return &restored, nil
}

func redactObjectiveDerivedModelText(
	authority turnAuthority,
	label string,
	value string,
) (string, error) {
	provenance, err := objectiveModelProvenance(authority)
	if err != nil {
		return "", err
	}
	bindings := make(map[string]string, len(authority.ModelArtifactIdentities))
	for _, identity := range authority.ModelArtifactIdentities {
		if _, duplicate := bindings[identity.Value]; duplicate {
			return "", fmt.Errorf("%s has duplicate binding for %q", label, identity.Value)
		}
		bindings[identity.Value] = identity.Token
	}
	matches := modelcontext.PathIdentities(value, provenance)
	if len(matches) == 0 {
		if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
			label, provenance, value,
		); err != nil {
			return "", err
		}
		return value, nil
	}
	var projected strings.Builder
	previous := 0
	for _, match := range matches {
		token, exists := bindings[match.Value]
		if !exists {
			return "", fmt.Errorf(
				"%s contains artifact identity %q absent from the exact instruction bindings",
				label, match.Value,
			)
		}
		projected.WriteString(value[previous:match.Start])
		projected.WriteString(token)
		previous = match.End
	}
	projected.WriteString(value[previous:])
	result := projected.String()
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		label, provenance, result,
	); err != nil {
		return "", err
	}
	return result, nil
}

func projectObjectiveRoleplayUserTurn(
	authority turnAuthority,
	userTurn roleplay.UserTurnAuthority,
) (roleplay.UserTurnAuthority, error) {
	if err := userTurn.Validate(); err != nil {
		return roleplay.UserTurnAuthority{}, err
	}
	projected := userTurn
	projected.Parts = append([]roleplay.UserTurnPart(nil), userTurn.Parts...)
	for index := range projected.Parts {
		text, err := redactObjectiveDerivedModelText(
			authority,
			fmt.Sprintf("roleplay user turn part %d", index),
			projected.Parts[index].Text,
		)
		if err != nil {
			return roleplay.UserTurnAuthority{}, err
		}
		projected.Parts[index].Text = text
	}
	if len(projected.Parts) == 0 {
		projected.ExactText = authority.ModelRedactedInstruction
	} else {
		exact, err := roleplay.ComposeUserTurn(roleplay.UserTurnRequest{
			PersonaKind: projected.PersonaKind, CharacterID: projected.CharacterID,
			ContributionKind: projected.ContributionKind, Parts: projected.Parts,
		})
		if err != nil {
			return roleplay.UserTurnAuthority{}, fmt.Errorf(
				"compose projected roleplay user turn: %w", err,
			)
		}
		projected.ExactText = exact
	}
	if err := projected.Validate(); err != nil {
		return roleplay.UserTurnAuthority{}, fmt.Errorf(
			"validate projected roleplay user turn: %w", err,
		)
	}
	if err := validateObjectiveModelInput(
		authority, "projected roleplay user turn",
		projected.PersonaName, projected.PersonaSummary, projected.ExactText,
	); err != nil {
		return roleplay.UserTurnAuthority{}, err
	}
	return projected, nil
}
