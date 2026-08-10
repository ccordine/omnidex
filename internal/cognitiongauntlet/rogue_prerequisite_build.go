package cognitiongauntlet

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/queue"
)

func NewRogueExtendedPrerequisite(
	source ExtendedRuntimeReceipt,
) (RoguePrerequisiteReceipt, error) {
	if err := source.Validate(); err != nil || source.Seal.Outcome != queue.CognitionEpisodeCompleted ||
		!containsSuite([]Suite{SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder}, source.Authority.Suite) {
		return RoguePrerequisiteReceipt{}, fmt.Errorf("Rogue extended prerequisite is not a completed isolated receipt")
	}
	authoritySHA, err := digestJSON(source.Authority)
	if err != nil {
		return RoguePrerequisiteReceipt{}, err
	}
	return newRoguePrerequisite(
		source.Authority.Suite, RogueSourceExtendedRuntime, authoritySHA,
		[]string{source.Seal.TraceSHA256}, source.ReceiptSHA256,
	)
}

func NewRogueResumePrerequisite(
	source FullCognitionRunResult,
) (RoguePrerequisiteReceipt, error) {
	if err := source.Validate(); err != nil || !source.Evaluation.GoalSuccess ||
		!source.Evaluation.ValidTerminalState || source.Episode.Manifest.Recovery.Restarts == 0 {
		return RoguePrerequisiteReceipt{}, fmt.Errorf("Rogue resume prerequisite lacks a successful sealed restart")
	}
	authoritySHA, err := digestJSON(source.Authority)
	if err != nil {
		return RoguePrerequisiteReceipt{}, err
	}
	qualification, err := digestJSON(struct {
		Evaluation string          `json:"evaluation_sha256"`
		Recovery   RecoveryMetrics `json:"recovery"`
	}{source.Evaluation.EpisodeSealSHA256, source.Episode.Manifest.Recovery})
	if err != nil {
		return RoguePrerequisiteReceipt{}, err
	}
	return newRoguePrerequisite(
		SuiteResume, RogueSourceResumeRuntime, authoritySHA,
		[]string{source.Episode.SealSHA256}, qualification,
	)
}

func NewRogueScalePrerequisite(
	source FullCognitionScaleResult,
) (RoguePrerequisiteReceipt, error) {
	if err := source.Validate(); err != nil || !source.Report.Gate.Passed {
		return RoguePrerequisiteReceipt{}, fmt.Errorf("Rogue scale prerequisite has not passed its sealed family gate")
	}
	artifacts, err := successfulScaleArtifacts(source)
	if err != nil {
		return RoguePrerequisiteReceipt{}, err
	}
	authoritySHA, err := digestJSON(source.Authority)
	if err != nil {
		return RoguePrerequisiteReceipt{}, err
	}
	qualification, err := digestJSON(source.Report)
	if err != nil {
		return RoguePrerequisiteReceipt{}, err
	}
	return newRoguePrerequisite(
		SuiteScale, RogueSourceScaleFamily, authoritySHA, artifacts, qualification,
	)
}

func NewRogueTransferPrerequisite(
	source FullCognitionTransferResult,
) (RoguePrerequisiteReceipt, error) {
	if err := source.Validate(); err != nil || !source.Report.Gate.Passed {
		return RoguePrerequisiteReceipt{}, fmt.Errorf("Rogue transfer prerequisite has not passed its sealed surface gate")
	}
	artifacts := make([]string, len(source.Runs))
	for index, run := range source.Runs {
		if !run.Evaluation.GoalSuccess || !run.Evaluation.ValidTerminalState {
			return RoguePrerequisiteReceipt{}, fmt.Errorf("Rogue transfer prerequisite contains an unsuccessful run")
		}
		artifacts[index] = run.Episode.SealSHA256
	}
	authoritySHA, err := digestJSON(source.Authority)
	if err != nil {
		return RoguePrerequisiteReceipt{}, err
	}
	qualification, err := digestJSON(source.Report)
	if err != nil {
		return RoguePrerequisiteReceipt{}, err
	}
	return newRoguePrerequisite(
		SuiteTransfer, RogueSourceTransferFamily, authoritySHA, artifacts, qualification,
	)
}

func successfulScaleArtifacts(source FullCognitionScaleResult) ([]string, error) {
	artifacts := make([]string, 0)
	for _, item := range source.Cases {
		for _, run := range item.Runs {
			if !run.Evaluation.GoalSuccess || !run.Evaluation.ValidTerminalState {
				return nil, fmt.Errorf("Rogue scale prerequisite contains an unsuccessful run")
			}
			artifacts = append(artifacts, run.Episode.SealSHA256)
		}
	}
	return artifacts, nil
}

func newRoguePrerequisite(
	suite Suite,
	source RoguePrerequisiteSource,
	authoritySHA string,
	artifacts []string,
	qualificationSHA string,
) (RoguePrerequisiteReceipt, error) {
	artifacts = append([]string(nil), artifacts...)
	sort.Strings(artifacts)
	receipt := RoguePrerequisiteReceipt{
		Schema: RoguePrerequisiteReceiptSchemaV1, Suite: suite, Source: source,
		AuthoritySHA256: authoritySHA, SealedArtifactSHA256s: artifacts,
		QualificationSHA256: qualificationSHA, PromotionEligible: false,
	}
	var err error
	receipt.ReceiptSHA256, err = roguePrerequisiteReceiptSHA(receipt)
	if err != nil {
		return RoguePrerequisiteReceipt{}, err
	}
	return receipt, receipt.Validate()
}

func NewRoguePrerequisiteBundle(
	receipts []RoguePrerequisiteReceipt,
) (RoguePrerequisiteBundle, error) {
	bundle := RoguePrerequisiteBundle{
		Schema:         RoguePrerequisiteBundleSchemaV1,
		FixtureVersion: ExtendedSuiteFixtureVersionV1,
		Receipts:       append([]RoguePrerequisiteReceipt(nil), receipts...),
	}
	for index := range bundle.Receipts {
		bundle.Receipts[index].SealedArtifactSHA256s = append(
			[]string(nil), bundle.Receipts[index].SealedArtifactSHA256s...,
		)
	}
	var err error
	bundle.BundleSHA256, err = roguePrerequisiteBundleSHA(bundle)
	if err != nil {
		return RoguePrerequisiteBundle{}, err
	}
	return bundle, bundle.Validate()
}
