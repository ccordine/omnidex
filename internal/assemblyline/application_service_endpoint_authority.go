package assemblyline

import "fmt"

type ApplicationServiceEndpointTaskAuthority struct {
	Surface          ApplicationSurface `json:"surface"`
	ProductContext   string             `json:"product_context"`
	RequirementQuote string             `json:"requirement_quote"`
}

func ProjectApplicationServiceEndpointTaskAuthority(
	authority ApplicationTaskRuntimeAuthority,
) (ApplicationServiceEndpointTaskAuthority, error) {
	projected := ApplicationServiceEndpointTaskAuthority{
		Surface: authority.Surface, ProductContext: authority.ProductQuote,
		RequirementQuote: authority.RequirementQuote,
	}
	if err := projected.validate(); err != nil {
		return ApplicationServiceEndpointTaskAuthority{}, err
	}
	return projected, nil
}

func (authority ApplicationServiceEndpointTaskAuthority) validate() error {
	switch authority.Surface {
	case ApplicationSurfaceBrowser, ApplicationSurfaceService:
	default:
		return fmt.Errorf("service endpoint application surface %q is unsupported", authority.Surface)
	}
	if err := validateApplicationProductQuote(
		"service endpoint product", authority.ProductContext,
	); err != nil {
		return err
	}
	if err := validateApplicationIntentText(
		"service endpoint requirement", authority.RequirementQuote, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	return nil
}
