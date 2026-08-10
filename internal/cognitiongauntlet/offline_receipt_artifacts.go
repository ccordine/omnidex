package cognitiongauntlet

import "fmt"

func (receipt OfflinePromotionReceipt) VerifyEvaluationArtifact(
	path string,
) (Evaluation, error) {
	if err := receipt.Validate(); err != nil {
		return Evaluation{}, err
	}
	return verifyReceiptEvaluation(
		path, receipt.EvaluationArtifactSHA256, receipt.EvaluationOracleSHA256,
	)
}

func (receipt OfflineTakeoverReceipt) VerifyEvaluationArtifact(
	path string,
) (Evaluation, error) {
	if err := receipt.Validate(); err != nil {
		return Evaluation{}, err
	}
	return verifyReceiptEvaluation(
		path, receipt.EvaluationArtifactSHA256, receipt.EvaluationOracleSHA256,
	)
}

func verifyReceiptEvaluation(
	path string,
	expectedArtifactSHA256 string,
	expectedOracleSHA256 string,
) (Evaluation, error) {
	evaluation, artifactSHA256, err := LoadEvaluationArtifact(path)
	if err != nil {
		return Evaluation{}, err
	}
	if artifactSHA256 != expectedArtifactSHA256 ||
		evaluation.OracleSHA256 != expectedOracleSHA256 {
		return Evaluation{}, fmt.Errorf("offline receipt evaluation artifact changed")
	}
	return evaluation, nil
}
