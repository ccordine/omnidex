package objectiveadvisory

import "fmt"

type Mode string

const (
	ModeOff    Mode = "off"
	ModeShadow Mode = "shadow"
	ModeActive Mode = "active"
)

func ParseMode(value string) (Mode, error) {
	if value == "" {
		return ModeOff, nil
	}
	mode := Mode(value)
	if err := mode.Validate(); err != nil {
		return "", err
	}
	return mode, nil
}

func (mode Mode) Validate() error {
	switch mode {
	case ModeOff, ModeShadow, ModeActive:
		return nil
	default:
		return fmt.Errorf("objective advisory mode %q is unsupported; expected off, shadow, or active", mode)
	}
}
