package database

import _ "embed"

// setupSQL is the sole authoritative runtime database definition.
//
//go:embed setup.sql
var setupSQL []byte

// SetupSQL returns an isolated copy of the checked-in database definition.
func SetupSQL() []byte {
	return append([]byte(nil), setupSQL...)
}
