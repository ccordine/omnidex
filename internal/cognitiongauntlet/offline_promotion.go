package cognitiongauntlet

import "context"

func RunOfflinePromotion(
	ctx context.Context,
	config OfflinePromotionConfig,
	executable string,
) (OfflinePromotionReceipt, error) {
	inference, err := runOfflinePromotionInference(ctx, config, executable)
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	return inference.evaluate(ctx)
}
