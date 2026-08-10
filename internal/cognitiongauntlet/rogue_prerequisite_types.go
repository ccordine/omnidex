package cognitiongauntlet

import "fmt"

const (
	RoguePrerequisiteReceiptSchemaV1 = "omnidex.rogue-prerequisite-receipt.v1"
	RoguePrerequisiteBundleSchemaV1  = "omnidex.rogue-prerequisite-bundle.v1"
)

type RoguePrerequisiteSource string

const (
	RogueSourceExtendedRuntime RoguePrerequisiteSource = "extended_runtime"
	RogueSourceResumeRuntime   RoguePrerequisiteSource = "resume_runtime"
	RogueSourceScaleFamily     RoguePrerequisiteSource = "scale_family"
	RogueSourceTransferFamily  RoguePrerequisiteSource = "transfer_family"
)

type RoguePrerequisiteReceipt struct {
	Schema                string                  `json:"schema"`
	Suite                 Suite                   `json:"suite"`
	Source                RoguePrerequisiteSource `json:"source"`
	AuthoritySHA256       string                  `json:"authority_sha256"`
	SealedArtifactSHA256s []string                `json:"sealed_artifact_sha256s"`
	QualificationSHA256   string                  `json:"qualification_sha256"`
	PromotionEligible     bool                    `json:"promotion_eligible"`
	ReceiptSHA256         string                  `json:"receipt_sha256"`
}

type RoguePrerequisiteBundle struct {
	Schema         string                     `json:"schema"`
	FixtureVersion string                     `json:"fixture_version"`
	Receipts       []RoguePrerequisiteReceipt `json:"receipts"`
	BundleSHA256   string                     `json:"bundle_sha256"`
}

func (receipt RoguePrerequisiteReceipt) Validate() error {
	if receipt.Schema != RoguePrerequisiteReceiptSchemaV1 ||
		!rogueSourceMatchesSuite(receipt.Source, receipt.Suite) ||
		!validDigest(receipt.AuthoritySHA256) || !validDigest(receipt.QualificationSHA256) ||
		receipt.SealedArtifactSHA256s == nil || len(receipt.SealedArtifactSHA256s) == 0 ||
		receipt.PromotionEligible {
		return fmt.Errorf("Rogue prerequisite receipt authority is invalid")
	}
	previous := ""
	for _, digest := range receipt.SealedArtifactSHA256s {
		if !validDigest(digest) || digest <= previous {
			return fmt.Errorf("Rogue prerequisite sealed artifacts must be unique and sorted")
		}
		previous = digest
	}
	want, err := roguePrerequisiteReceiptSHA(receipt)
	if err != nil || receipt.ReceiptSHA256 != want {
		return fmt.Errorf("Rogue prerequisite receipt hash changed")
	}
	return nil
}

func (bundle RoguePrerequisiteBundle) Validate() error {
	if bundle.Schema != RoguePrerequisiteBundleSchemaV1 ||
		bundle.FixtureVersion != ExtendedSuiteFixtureVersionV1 || bundle.Receipts == nil {
		return fmt.Errorf("Rogue prerequisite bundle authority is invalid")
	}
	required := roguePrerequisites()
	if len(bundle.Receipts) != len(required) {
		return fmt.Errorf("Rogue prerequisite bundle is incomplete")
	}
	for index, receipt := range bundle.Receipts {
		if receipt.Suite != required[index] || receipt.Validate() != nil {
			return fmt.Errorf("Rogue prerequisite receipt %d is invalid or out of order", index+1)
		}
	}
	want, err := roguePrerequisiteBundleSHA(bundle)
	if err != nil || bundle.BundleSHA256 != want {
		return fmt.Errorf("Rogue prerequisite bundle hash changed")
	}
	return nil
}

func rogueSourceMatchesSuite(source RoguePrerequisiteSource, suite Suite) bool {
	switch source {
	case RogueSourceExtendedRuntime:
		return containsSuite([]Suite{SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder}, suite)
	case RogueSourceResumeRuntime:
		return suite == SuiteResume
	case RogueSourceScaleFamily:
		return suite == SuiteScale
	case RogueSourceTransferFamily:
		return suite == SuiteTransfer
	default:
		return false
	}
}

func roguePrerequisiteReceiptSHA(receipt RoguePrerequisiteReceipt) (string, error) {
	receipt.ReceiptSHA256 = ""
	return digestJSON(receipt)
}

func roguePrerequisiteBundleSHA(bundle RoguePrerequisiteBundle) (string, error) {
	bundle.BundleSHA256 = ""
	return digestJSON(bundle)
}
