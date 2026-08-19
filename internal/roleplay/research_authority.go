package roleplay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const ResearchTurnAuthoritySchemaV1 = "omnidex.roleplay-research-turn.v1"

var (
	ErrResearchAuthorityConflict = errors.New("roleplay research authority conflict")
	ErrResearchAuthorityAbsent   = errors.New("roleplay research authority is absent")
)

type ResearchTurnAuthority struct {
	Schema               string              `json:"schema"`
	PreparationID        string              `json:"preparation_id"`
	ChannelID            string              `json:"channel_id"`
	UserMessageID        int64               `json:"user_message_id"`
	WorldID              string              `json:"world_id"`
	SceneID              string              `json:"scene_id"`
	SceneRevision        int64               `json:"scene_revision"`
	CharacterID          string              `json:"character_id"`
	Capability           CharacterCapability `json:"capability"`
	CapabilityGrantID    string              `json:"capability_grant_id"`
	Question             string              `json:"question"`
	QuestionSHA256       string              `json:"question_sha256"`
	NarrativeFingerprint string              `json:"narrative_fingerprint"`
	Authority            AuthorityNamespace  `json:"authority_namespace"`
	CapabilityIssuedAt   time.Time           `json:"capability_issued_at"`
	CreatedAt            time.Time           `json:"created_at"`
}

func (authority ResearchTurnAuthority) Validate() error {
	if authority.Schema != ResearchTurnAuthoritySchemaV1 || authority.Authority != AuthorityRealWorld ||
		authority.Capability != CapabilityWebResearch {
		return fmt.Errorf("roleplay research authority has invalid schema or namespace")
	}
	if validateIdentity(authority.PreparationID, transitionIdentity) != nil ||
		validateChannelID(authority.ChannelID) != nil || authority.UserMessageID < 1 ||
		validateIdentity(authority.WorldID, worldIdentity) != nil ||
		validateIdentity(authority.SceneID, sceneIdentity) != nil ||
		validateIdentity(authority.CharacterID, characterIdentity) != nil ||
		validateIdentity(authority.CapabilityGrantID, capabilityGrantIdentity) != nil ||
		authority.SceneRevision < 1 || !validSimulationSHA(authority.NarrativeFingerprint) ||
		authority.CapabilityIssuedAt.IsZero() || authority.CreatedAt.IsZero() {
		return fmt.Errorf("roleplay research authority has invalid identity, revision, fingerprint, or time")
	}
	if err := validateResearchQuestion(authority.Question); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(authority.Question))
	if authority.QuestionSHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("roleplay research question differs from its exact SHA-256")
	}
	return nil
}

func AuthorizeResearchPreparationTx(
	ctx context.Context,
	tx pgx.Tx,
	preparation SimulationTurnAuthority,
	command ResearchCommand,
) (ResearchTurnAuthority, error) {
	if ctx == nil || tx == nil {
		return ResearchTurnAuthority{}, fmt.Errorf("roleplay research authorization requires transaction authority")
	}
	if err := preparation.Validate(); err != nil {
		return ResearchTurnAuthority{}, err
	}
	if preparation.InputKind != SimulationTurnExternalCommand || preparation.ExplicitAction {
		return ResearchTurnAuthority{}, fmt.Errorf("roleplay research requires external-command preparation authority")
	}
	parsed, matched, err := ParseResearchCommand(command.Exact)
	if err != nil || !matched {
		if err != nil {
			return ResearchTurnAuthority{}, err
		}
		return ResearchTurnAuthority{}, fmt.Errorf("roleplay research authorization requires exact reserved command syntax")
	}
	if parsed != command {
		return ResearchTurnAuthority{}, fmt.Errorf("roleplay research command differs from its canonical parse")
	}
	questionDigest := sha256.Sum256([]byte(command.Question))
	questionSHA := hex.EncodeToString(questionDigest[:])
	if existing, found, err := loadResearchPreparationTx(ctx, tx, preparation.PreparationID); err != nil || found {
		if err != nil {
			return ResearchTurnAuthority{}, err
		}
		if existing.ChannelID != preparation.ChannelID || existing.UserMessageID != preparation.UserMessageID ||
			existing.WorldID != preparation.WorldID || existing.SceneID != preparation.SceneID ||
			existing.SceneRevision != preparation.SceneRevision ||
			existing.CharacterID != preparation.ActiveCharacterID || existing.Question != command.Question ||
			existing.QuestionSHA256 != questionSHA ||
			existing.NarrativeFingerprint != preparation.NarrativeFingerprint {
			return ResearchTurnAuthority{}, fmt.Errorf("%w: preparation was reused with different research authority", ErrResearchAuthorityConflict)
		}
		return existing, nil
	}
	var authority ResearchTurnAuthority
	var namespace string
	err = tx.QueryRow(ctx, `
		INSERT INTO roleplay_research_turns (
			preparation_id,channel_id,user_message_id,world_id,scene_id,scene_revision,
			character_id,capability,capability_grant_id,question,question_sha256,
			narrative_fingerprint
		)
		SELECT preparation.operation_id,preparation.channel_id,preparation.user_message_id,
		       preparation.world_id,preparation.scene_id,preparation.scene_revision,
		       preparation.active_character_id,capability.capability,capability.grant_id,
		       $2,$3,preparation.result->>'narrative_fingerprint'
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN ai_channel_messages AS message
		  ON message.channel_id=preparation.channel_id
		 AND message.id=preparation.user_message_id
		 AND message.role='user'
		JOIN roleplay_character_capabilities AS capability
		  ON capability.world_id=preparation.world_id
		 AND capability.character_id=preparation.active_character_id
		 AND capability.capability='web_research'
		WHERE preparation.operation_id=$1
		  AND preparation.channel_id=$4 AND preparation.user_message_id=$5
		  AND preparation.world_id=$6 AND preparation.scene_id=$7
		  AND preparation.scene_revision=$8 AND preparation.active_character_id=$9
		  AND preparation.input_kind='external_command' AND NOT preparation.explicit_action
		  AND preparation.result->>'narrative_fingerprint'=$10 AND message.content=$11
		RETURNING $12,preparation_id,channel_id,user_message_id,world_id,scene_id,
		          scene_revision,character_id,capability,capability_grant_id,question,
		          question_sha256,narrative_fingerprint,authority_namespace,
		          (SELECT created_at FROM roleplay_character_capability_grants
		           WHERE grant_id=capability_grant_id),created_at
	`, preparation.PreparationID, command.Question, questionSHA, preparation.ChannelID,
		preparation.UserMessageID, preparation.WorldID, preparation.SceneID,
		preparation.SceneRevision, preparation.ActiveCharacterID, preparation.NarrativeFingerprint,
		command.Exact, ResearchTurnAuthoritySchemaV1).Scan(
		&authority.Schema, &authority.PreparationID, &authority.ChannelID, &authority.UserMessageID,
		&authority.WorldID, &authority.SceneID, &authority.SceneRevision, &authority.CharacterID,
		&authority.Capability, &authority.CapabilityGrantID, &authority.Question,
		&authority.QuestionSHA256, &authority.NarrativeFingerprint, &namespace,
		&authority.CapabilityIssuedAt, &authority.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return ResearchTurnAuthority{}, fmt.Errorf("%w: active character has no exact persisted grant", ErrResearchCapabilityDenied)
	}
	if err != nil {
		return ResearchTurnAuthority{}, fmt.Errorf("persist roleplay research authorization: %w", err)
	}
	authority.Authority = AuthorityNamespace(namespace)
	if err := authority.Validate(); err != nil {
		return ResearchTurnAuthority{}, err
	}
	return authority, nil
}

func BindResearchPreparationJobTx(
	ctx context.Context,
	tx pgx.Tx,
	preparationID string,
	jobID int64,
) error {
	if ctx == nil || tx == nil {
		return fmt.Errorf("roleplay research job binding requires transaction authority")
	}
	if err := validateIdentity(preparationID, transitionIdentity); err != nil {
		return err
	}
	if jobID < 1 {
		return fmt.Errorf("roleplay research job binding requires a positive job ID")
	}
	var existing int64
	err := tx.QueryRow(ctx, `
		SELECT job_id FROM roleplay_research_preparation_jobs WHERE preparation_id=$1
	`, preparationID).Scan(&existing)
	if err == nil {
		if existing != jobID {
			return fmt.Errorf("%w: research preparation is bound to another job", ErrResearchAuthorityConflict)
		}
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO roleplay_research_preparation_jobs (preparation_id,job_id)
		SELECT research.preparation_id,binding.job_id
		FROM roleplay_research_turns AS research
		JOIN roleplay_simulation_preparation_jobs AS binding
		  ON binding.preparation_id=research.preparation_id
		JOIN jobs AS job ON job.id=binding.job_id
		WHERE research.preparation_id=$1 AND binding.job_id=$2
		  AND job.pipeline='chat' AND job.instruction=(
		      SELECT content FROM ai_channel_messages WHERE id=research.user_message_id
		  )
	`, preparationID, jobID)
	if err != nil {
		return fmt.Errorf("bind roleplay research job: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: research preparation and simulation job binding differ", ErrResearchAuthorityAbsent)
	}
	return nil
}

func (s *Store) LoadResearchTurnForJob(
	ctx context.Context,
	jobID int64,
) (ResearchTurnAuthority, error) {
	if err := s.validateContext(ctx); err != nil {
		return ResearchTurnAuthority{}, err
	}
	if jobID < 1 {
		return ResearchTurnAuthority{}, fmt.Errorf("roleplay research lookup requires a positive job ID")
	}
	authority, found, err := loadResearchJob(ctx, s.pool, jobID, true)
	if err != nil {
		return ResearchTurnAuthority{}, err
	}
	if !found {
		return ResearchTurnAuthority{}, ErrResearchAuthorityAbsent
	}
	return authority, nil
}

func FindResearchTurnForJobTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
) (ResearchTurnAuthority, bool, error) {
	if ctx == nil || tx == nil {
		return ResearchTurnAuthority{}, false, fmt.Errorf("roleplay research lookup requires transaction authority")
	}
	if jobID < 1 {
		return ResearchTurnAuthority{}, false, fmt.Errorf("roleplay research lookup requires a positive job ID")
	}
	return loadResearchJob(ctx, tx, jobID, true)
}

// FindResearchTurnBindingForJobTx identifies a persisted research job even
// after its active grant was revoked. Completion code uses this to fail the
// research turn instead of allowing it to fall through to fictional canon.
func FindResearchTurnBindingForJobTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
) (ResearchTurnAuthority, bool, error) {
	if ctx == nil || tx == nil {
		return ResearchTurnAuthority{}, false, fmt.Errorf("roleplay research binding lookup requires transaction authority")
	}
	if jobID < 1 {
		return ResearchTurnAuthority{}, false, fmt.Errorf("roleplay research binding lookup requires a positive job ID")
	}
	return loadResearchJob(ctx, tx, jobID, false)
}

func validateResearchQuestion(question string) error {
	command, matched, err := ParseResearchCommand("/research " + fmt.Sprintf("%q", question))
	if err != nil || !matched || command.Question != question {
		return fmt.Errorf("roleplay research question is not a canonical bounded value")
	}
	return nil
}
