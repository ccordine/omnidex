package main

import "strings"

// These runes are rejected by input validation. Only the paste reader may
// introduce them, so they cannot collide with accepted literal user text.
// Each occupies one editor position, preserving cursor and deletion behavior.
const (
	pasteLineFeed       = '\u2028'
	pasteCarriageReturn = '\u2029'
	pasteTab            = '\u0085'
)

var (
	restorePasteProjection = strings.NewReplacer(
		string(pasteLineFeed), "\n",
		string(pasteCarriageReturn), "\r",
		string(pasteTab), "\t",
	)
	displayPasteProjection = strings.NewReplacer(
		string(pasteLineFeed), "↵",
		string(pasteCarriageReturn), "␍",
		string(pasteTab), "⇥",
	)
)

func projectPasteRune(character rune) rune {
	switch character {
	case '\n':
		return pasteLineFeed
	case '\r':
		return pasteCarriageReturn
	case '\t':
		return pasteTab
	default:
		return character
	}
}
