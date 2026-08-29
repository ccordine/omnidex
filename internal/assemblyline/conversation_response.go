package assemblyline

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	ConversationResponseSchemaV1     = "omnidex.conversation-response.v1"
	maxConversationResponseTextBytes = 8 * 1024
)

type ConversationResponseInput struct {
	Kind               ConversationObjectiveKind   `json:"kind"`
	ExactInstruction   string                      `json:"exact_instruction"`
	Context            ObjectiveContext            `json:"objective_context"`
	RoleplayIdentity   *RoleplayResponseIdentity   `json:"roleplay_identity"`
	RoleplayUserTurn   *RoleplayUserTurnProjection `json:"roleplay_user_turn"`
	KnownArtifactPaths []string                    `json:"known_artifact_paths"`
}

type RoleplayResponseIdentity struct {
	CharacterName string `json:"character_name"`
	Summary       string `json:"summary"`
	Voice         string `json:"voice"`
}

type ConversationResponseDecision struct {
	Schema string `json:"schema"`
	Text   string `json:"text"`
}

func NewConversationResponseJob(input ConversationResponseInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkConversationResponse, input, input.validate)
}

func (input ConversationResponseInput) validate() error {
	provenance, err := modelcontext.NewArtifactIdentityProvenance(input.KnownArtifactPaths)
	if err != nil {
		return fmt.Errorf("conversation response artifact provenance: %w", err)
	}
	if input.Kind != ObjectiveKindAnswer && input.Kind != ObjectiveKindStory {
		return fmt.Errorf("conversation response kind %q is unsupported", input.Kind)
	}
	if input.RoleplayIdentity != nil {
		if input.Kind != ObjectiveKindStory {
			return fmt.Errorf("fictional character authority is valid only for a story response")
		}
		for label, value := range map[string]string{
			"character name":    input.RoleplayIdentity.CharacterName,
			"character summary": input.RoleplayIdentity.Summary,
		} {
			if err := validateContextText("roleplay "+label, value, 1024); err != nil {
				return err
			}
		}
		if err := validateOptionalContextText(
			"roleplay character voice", input.RoleplayIdentity.Voice, 1024,
		); err != nil {
			return err
		}
		if input.RoleplayUserTurn == nil {
			return fmt.Errorf("roleplay response requires exact user persona and contribution authority")
		}
		if err := input.RoleplayUserTurn.validate(); err != nil {
			return err
		}
		values := []string{
			input.RoleplayIdentity.CharacterName,
			input.RoleplayIdentity.Summary,
			input.RoleplayIdentity.Voice,
			input.RoleplayUserTurn.PersonaName,
			input.RoleplayUserTurn.PersonaSummary,
		}
		for _, part := range input.RoleplayUserTurn.Parts {
			values = append(values, part.Text)
		}
		if err := ValidatePathFreeModelContextWithProvenance(
			"conversation roleplay authority", provenance, values...,
		); err != nil {
			return err
		}
	} else if input.RoleplayUserTurn != nil {
		return fmt.Errorf("roleplay user turn requires one responding character identity")
	}
	return (ConversationObjectiveKindInput{
		ExactInstruction:   input.ExactInstruction,
		Context:            input.Context,
		KnownArtifactPaths: append([]string{}, input.KnownArtifactPaths...),
	}).validate()
}

func (decision ConversationResponseDecision) ValidateFor(input ConversationResponseInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ConversationResponseSchemaV1 {
		return fmt.Errorf("conversation response schema must be %q", ConversationResponseSchemaV1)
	}
	maxBytes := maxConversationResponseTextBytes
	if input.RoleplayIdentity != nil {
		maxBytes = roleplay.MaxNarrativeResponseBytes
	}
	if err := validateGroundedText("conversation response text", decision.Text, maxBytes, true); err != nil {
		return err
	}
	if input.RoleplayIdentity != nil {
		if err := validateRoleplayProse("conversation response text", decision.Text); err != nil {
			return err
		}
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(input.KnownArtifactPaths)
	if err != nil {
		return err
	}
	return decision.ValidatePathFree(provenance)
}

func (decision ConversationResponseDecision) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	return ValidatePathFreeModelContextWithProvenance(
		"conversation response text", provenance, decision.Text,
	)
}

func DecodeConversationResponseDecision(
	input ConversationResponseInput,
	raw string,
) (ConversationResponseDecision, error) {
	maximum := maxConversationResponseTextBytes
	if input.RoleplayIdentity != nil {
		maximum = roleplay.MaxNarrativeResponseBytes
	}
	leaf, err := decodeRawSemanticLeaf("conversation response", raw, maximum, true)
	if err != nil {
		return ConversationResponseDecision{}, err
	}
	decision := ConversationResponseDecision{Schema: ConversationResponseSchemaV1, Text: leaf}
	if err := decision.ValidateFor(input); err != nil {
		return ConversationResponseDecision{}, err
	}
	return decision, nil
}

func BuildConversationResponsePrompt(input ConversationResponseInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	modelContext, err := projectObjectiveContextForModel(input.Context)
	if err != nil {
		return "", err
	}
	context, err := json.Marshal(modelContext)
	if err != nil {
		return "", fmt.Errorf("encode objective context: %w", err)
	}
	sections := []string{"Answer exactly one user instruction.",
		"Return one bounded response text leaf that directly satisfies that instruction using only the supplied context.",
		"Return only the raw response text with no JSON, quotes, label, Markdown wrapper, or commentary outside the response itself.",
		"OBJECTIVE_CONTEXT_JSON:\n" + string(context)}
	if input.RoleplayIdentity != nil {
		identity, err := json.Marshal(input.RoleplayIdentity)
		if err != nil {
			return "", fmt.Errorf("encode roleplay character identity: %w", err)
		}
		userTurn, err := json.Marshal(input.RoleplayUserTurn)
		if err != nil {
			return "", fmt.Errorf("encode roleplay user turn: %w", err)
		}
		responderName := strconv.Quote(input.RoleplayIdentity.CharacterName)
		personaName := strconv.Quote(input.RoleplayUserTurn.PersonaName)
		sections = []string{
			"Write one in-character narrative response to exactly one user turn.",
			"Return only the raw narrative text with no JSON, quotes, label, Markdown wrapper, or commentary outside the narrative.",
			"The responding character is " + responderName + "; the user-controlled persona is " + personaName + ". These are distinct narrative identities.",
			"The exact user turn controls the immediate response. Respond to what the user just said or did; background state supports the response and must not replace it with an unrelated continuation.",
			"Begin with the character's direct reaction or reply to the current user turn before advancing any other scene beat. When the user speaks, answer or acknowledge that speech in character.",
			"Treat the compact objective context as background constraint only; it must not replace a direct response to the current turn.",
			"A short user turn permits a short response. Do not invent a different request merely to use background details.",
			"Keep the prose consistent with the supplied fictional reality and already-applied recent events.",
			"When the compact objective context includes an earlier response from this ordered round, it happened after the user turn. React after it without changing its words or speaking for that character.",
			roleplayContributionInstruction(*input.RoleplayUserTurn, responderName, personaName),
			fmt.Sprintf(
				"Keep the response text to one to three short paragraphs and no more than %d UTF-8 bytes. End as soon as the response is complete.",
				roleplay.MaxNarrativeResponseBytes,
			),
			"ROLEPLAY_IDENTITY_JSON:\n" + string(identity),
			"ROLEPLAY_USER_TURN_JSON:\n" + string(userTurn),
			"COMPILED_OBJECTIVE_CONTEXT_JSON:\n" + string(context),
		}
	}
	sections = append(sections, "EXACT_INSTRUCTION:\n"+input.ExactInstruction)
	return strings.Join(sections, "\n\n"), nil
}

func roleplayContributionInstruction(
	turn RoleplayUserTurnProjection,
	responderName string,
	personaName string,
) string {
	if len(turn.Parts) != 0 {
		return "Honor ROLEPLAY_USER_TURN_JSON.parts in their exact order. Each Message is speech by " +
			personaName + "; each Action is performed or attempted by " + personaName +
			"; each Event is narrator-established fictional reality. React as " + responderName +
			" without transferring the user's words, actions, or established events to the responder and without embellishing what the user supplied."
	}
	switch turn.ContributionKind {
	case roleplay.UserContributionDialogue:
		return "The exact instruction is speech by " + personaName + ". Answer that speech as " + responderName + "; do not quote or paraphrase it as the responder's own words."
	case roleplay.UserContributionAction:
		return "The exact instruction describes an action or attempt by " + personaName + ". React as " + responderName + ". Never narrate " + personaName + "'s action as " + responderName + "'s action, and do not embellish what the user did."
	case roleplay.UserContributionActionDialogue:
		return "Actions and quoted speech in the exact instruction belong to " + personaName + ". React as " + responderName + "; never transfer those actions or words to the responder."
	case roleplay.UserContributionNarration:
		return "The exact instruction is narrator-authored fictional narration. Treat explicitly stated events as established unless they contradict supplied authoritative state; react as " + responderName + " without rewriting them as the responder's actions."
	case roleplay.UserContributionDirection:
		return "The exact instruction is a narrator direction or question, not itself a fictional event. Fulfill it directly as " + responderName + " while preserving supplied state and recent continuity."
	case roleplay.UserContributionNarrationDirection:
		return "The exact instruction contains ordered narrator-authored Action and Message parts. Treat Action parts as established fictional events and Message parts as directions or questions. Fulfill them in order as " + responderName + " without transferring narrator-authored actions to the responder."
	case roleplay.UserContributionStructured:
		return "The exact instruction is an ordered structured turn. Preserve the distinct ownership of every message, action, and event while reacting as " + responderName + "."
	case roleplay.UserContributionCommand:
		return "The exact instruction describes the already-applied deterministic result of an explicit command. Continue from that supplied result as " + responderName + "."
	default:
		return ""
	}
}
