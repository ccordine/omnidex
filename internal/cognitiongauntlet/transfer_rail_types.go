package cognitiongauntlet

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

const TransferAuthoritySchemaV1 = "omnidex.cognition-transfer-authority.v1"

type RuntimeFingerprint struct {
	ProductionSourceSHA256 string `json:"production_source_sha256"`
	RendererSHA256         string `json:"renderer_sha256"`
	RetentionPolicySHA256  string `json:"retention_policy_sha256"`
	ObligationPolicySHA256 string `json:"obligation_policy_sha256"`
	PromptSHA256           string `json:"prompt_sha256"`
}

type TransferAuthority struct {
	Schema               string                `json:"schema"`
	CaseID               string                `json:"case_id"`
	TaskSuite            Suite                 `json:"task_suite"`
	FixtureVersion       string                `json:"fixture_version"`
	GeneratorVersion     string                `json:"generator_version"`
	Seed                 uint64                `json:"seed"`
	Scenario             cognition.ScenarioRef `json:"scenario"`
	OracleSHA256         string                `json:"oracle_sha256"`
	ActionCatalogVersion string                `json:"action_catalog_version"`
	ActionCatalogSHA256  string                `json:"action_catalog_sha256"`
	SurfaceVersions      []string              `json:"surface_versions"`
	Variant              Variant               `json:"variant"`
	Repetition           int                   `json:"repetition"`
	RatGeneration        RatGeneration         `json:"rat_generation"`
	Budget               RunBudget             `json:"budget"`
	Runtime              RuntimeFingerprint    `json:"runtime"`
}

type TransferEpisodeResult struct {
	AuthoritySHA256    string                  `json:"authority_sha256"`
	SurfaceVersion     string                  `json:"surface_version"`
	Variant            Variant                 `json:"variant"`
	EpisodeSealSHA256  string                  `json:"episode_seal_sha256"`
	GoalSuccess        bool                    `json:"goal_success"`
	CleanDeskQualified bool                    `json:"clean_desk_qualified"`
	CausalAcquisition  CausalAcquisitionReport `json:"causal_acquisition"`
}

type TransferRailReport struct {
	Authority TransferAuthority       `json:"authority"`
	Episodes  []TransferEpisodeResult `json:"episodes"`
	Gate      GateResult              `json:"gate"`
}

func (fingerprint RuntimeFingerprint) Validate() error {
	for _, digest := range []string{
		fingerprint.ProductionSourceSHA256, fingerprint.RendererSHA256,
		fingerprint.RetentionPolicySHA256, fingerprint.ObligationPolicySHA256,
		fingerprint.PromptSHA256,
	} {
		if !validDigest(digest) {
			return fmt.Errorf("cognition runtime fingerprint is invalid")
		}
	}
	return nil
}

func (authority TransferAuthority) Validate() error {
	if authority.Schema != TransferAuthoritySchemaV1 || !validSuite(authority.TaskSuite) {
		return fmt.Errorf("transfer authority schema or suite is invalid")
	}
	if !validVariant(authority.Variant) {
		return fmt.Errorf("transfer authority variant is invalid")
	}
	for label, value := range map[string]string{
		"transfer case ID":                authority.CaseID,
		"transfer fixture version":        authority.FixtureVersion,
		"transfer generator version":      authority.GeneratorVersion,
		"transfer action catalog version": authority.ActionCatalogVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if err := authority.Scenario.Validate(); err != nil {
		return err
	}
	if !validDigest(authority.OracleSHA256) || !validDigest(authority.ActionCatalogSHA256) {
		return fmt.Errorf("transfer oracle or action catalog hash is invalid")
	}
	if len(authority.SurfaceVersions) < 2 {
		return fmt.Errorf("transfer authority requires at least two surfaces")
	}
	previous := ""
	for _, version := range authority.SurfaceVersions {
		if err := requireExact(version, "transfer surface version", 256); err != nil {
			return err
		}
		if version <= previous {
			return fmt.Errorf("transfer surface versions must be uniquely sorted")
		}
		previous = version
	}
	if authority.Repetition <= 0 || authority.Repetition > 10_000 {
		return fmt.Errorf("transfer repetition is invalid")
	}
	if err := authority.Budget.ValidateFor(authority.RatGeneration); err != nil {
		return err
	}
	return authority.Runtime.Validate()
}

func sortedSurfaceVersions(surfaces []Surface) ([]string, error) {
	versions := make([]string, len(surfaces))
	for index, surface := range surfaces {
		version, err := surface.Version()
		if err != nil {
			return nil, err
		}
		versions[index] = version
	}
	sort.Strings(versions)
	return versions, nil
}
