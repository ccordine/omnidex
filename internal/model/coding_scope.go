package model

import "fmt"

type CodingScopeMode string

const (
	CodingScopeModeStrict    CodingScopeMode = "strict"
	CodingScopeModeNormal    CodingScopeMode = "normal"
	CodingScopeModeExpansive CodingScopeMode = "expansive"
)

func (mode CodingScopeMode) Validate() error {
	switch mode {
	case CodingScopeModeStrict, CodingScopeModeNormal, CodingScopeModeExpansive:
		return nil
	default:
		return fmt.Errorf("coding scope mode %q is unsupported", mode)
	}
}
