package assemblyline

import "regexp"

const (
	maxConstructionDocuments = 32
	maxLocalBehaviorBytes    = 2400
)

var (
	graphIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
	codeIdentifierPattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)
