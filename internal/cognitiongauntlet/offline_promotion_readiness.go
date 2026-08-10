package cognitiongauntlet

// seriousPromotionEvidenceReady stays false until the exact provider-evidence
// migration and checked release migration manifest are both frozen and proven
// through the real subprocess/PostgreSQL rails. A passing experimental gate is
// still diagnostic while either release authority is unavailable.
const seriousPromotionEvidenceReady = false

func seriousPromotionEligible(gatePassed bool) bool {
	return seriousPromotionEvidenceReady && gatePassed
}
