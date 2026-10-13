package main

type chatVerbosity uint8

const (
	chatNormal chatVerbosity = iota
	chatQuiet
	chatVerbose
	chatDebug
)

func (verbosity chatVerbosity) String() string {
	switch verbosity {
	case chatNormal:
		return "normal"
	case chatQuiet:
		return "quiet"
	case chatVerbose:
		return "verbose"
	case chatDebug:
		return "debug"
	default:
		return "invalid"
	}
}
