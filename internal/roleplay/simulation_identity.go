package roleplay

import "regexp"

var (
	sceneIdentity      = identityKind{name: "roleplay scene", pattern: regexp.MustCompile(`^rps_[0-9a-f]{32}$`)}
	commandIdentity    = identityKind{name: "roleplay interaction command", pattern: regexp.MustCompile(`^rpa_[0-9a-f]{32}$`)}
	itemIdentity       = identityKind{name: "roleplay item template", pattern: regexp.MustCompile(`^rpi_[0-9a-f]{32}$`)}
	inventoryIdentity  = identityKind{name: "roleplay inventory item", pattern: regexp.MustCompile(`^rpv_[0-9a-f]{32}$`)}
	transitionIdentity = identityKind{name: "roleplay simulation transition", pattern: regexp.MustCompile(`^rpt_[0-9a-f]{32}$`)}
	memoryIdentity     = identityKind{name: "roleplay character memory", pattern: regexp.MustCompile(`^rpm_[0-9a-f]{32}$`)}
)

func NewSceneIdentity() (string, error) { return newIdentity("rps_") }

func NewInteractionCommandIdentity() (string, error) { return newIdentity("rpa_") }

func NewItemTemplateIdentity() (string, error) { return newIdentity("rpi_") }

func NewSimulationTransitionIdentity() (string, error) { return newIdentity("rpt_") }
