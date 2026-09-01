package main

import (
	"bytes"
	"io"
	"unicode/utf8"
)

var (
	bracketedPasteStart = []byte("\x1b[200~")
	bracketedPasteEnd   = []byte("\x1b[201~")
)

// bracketedPasteReader keeps one terminal paste inside one line-editor turn.
// Terminals commonly encode pasted newlines as CR or CRLF; x/term otherwise
// treats each CR as submission even while its bracketed-paste state is active.
type bracketedPasteReader struct {
	source           io.Reader
	maxBytes         int
	paste            bool
	pasteCR          bool
	linePasted       bool
	lineMixed        bool
	lineBytes        int
	overflow         bool
	invalidUTF8      bool
	unsafeText       bool
	rawPaste         []byte
	pasteRune        []byte
	typedRune        []byte
	candidate        []byte
	queued           []byte
	rawQueued        []byte
	rawAuthority     terminalInputAuthority
	lineAuthority    terminalInputAuthority
	lineAuthoritySet bool
	skipLineLF       bool
	pendingErr       error
	buffer           [512]byte
}

func newBracketedPasteReader(source io.Reader, maxBytes int) *bracketedPasteReader {
	return &bracketedPasteReader{source: source, maxBytes: maxBytes}
}

func (reader *bracketedPasteReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	for len(reader.queued) == 0 {
		if reader.pendingErr != nil && len(reader.rawQueued) == 0 {
			err := reader.pendingErr
			reader.pendingErr = nil
			return 0, err
		}
		values := reader.rawQueued
		authority := reader.rawAuthority
		reader.rawQueued = nil
		reader.rawAuthority = terminalInputAuthority{}
		var err error
		if len(values) == 0 {
			count := 0
			if source, ok := reader.source.(interface {
				ReadWithAuthority([]byte) (int, terminalInputAuthority, error)
			}); ok {
				count, authority, err = source.ReadWithAuthority(reader.buffer[:])
			} else {
				count, err = reader.source.Read(reader.buffer[:])
			}
			values = reader.buffer[:count]
		}
		if len(values) > 0 {
			reader.mergeLineAuthority(authority)
		}
		for index, value := range values {
			if reader.consume(value) {
				reader.rawQueued = append(reader.rawQueued, values[index+1:]...)
				reader.rawAuthority = authority
				break
			}
		}
		if err != nil {
			reader.flushCandidate()
			if len(reader.queued) == 0 {
				return 0, err
			}
			reader.pendingErr = err
		}
		if len(values) == 0 && err == nil {
			continue
		}
	}
	count := copy(destination, reader.queued)
	reader.queued = reader.queued[count:]
	return count, nil
}

func (reader *bracketedPasteReader) consume(value byte) bool {
	if !reader.paste && reader.skipLineLF {
		reader.skipLineLF = false
		if value == '\n' {
			return false
		}
	}
	expected := bracketedPasteStart
	if reader.paste {
		expected = bracketedPasteEnd
	}
	reader.candidate = append(reader.candidate, value)
	for len(reader.candidate) > 0 && !bytes.HasPrefix(expected, reader.candidate) {
		reader.consumeLiteral(reader.candidate[0])
		reader.candidate = reader.candidate[1:]
	}
	if bytes.Equal(reader.candidate, expected) {
		if !reader.paste {
			reader.finishTypedRune()
		}
		if reader.paste {
			reader.finishPastePayload()
		}
		reader.pasteCR = false
		reader.queued = append(reader.queued, reader.candidate...)
		reader.candidate = reader.candidate[:0]
		reader.paste = !reader.paste
		if reader.paste {
			reader.linePasted = true
		}
	}
	lineEnd := !reader.paste && value == '\r' && len(reader.candidate) == 0
	if lineEnd {
		reader.skipLineLF = true
	}
	return lineEnd
}

func (reader *bracketedPasteReader) flushCandidate() {
	for _, value := range reader.candidate {
		reader.consumeLiteral(value)
	}
	reader.candidate = reader.candidate[:0]
}

func (reader *bracketedPasteReader) consumeLiteral(value byte) {
	if !reader.paste {
		reader.consumeTypedByte(value)
		if value != '\r' {
			reader.lineMixed = true
		}
		return
	}
	reader.consumePasteByte(value)
}

func (reader *bracketedPasteReader) consumeTypedByte(value byte) {
	if value < utf8.RuneSelf {
		reader.finishTypedRune()
		reader.queued = append(reader.queued, value)
		return
	}
	reader.typedRune = append(reader.typedRune, value)
	if !utf8.FullRune(reader.typedRune) {
		return
	}
	character, size := utf8.DecodeRune(reader.typedRune)
	if character == utf8.RuneError && size == 1 {
		reader.invalidUTF8 = true
		reader.typedRune = reader.typedRune[:0]
		return
	}
	if unsafeTerminalTextRune(character) {
		reader.unsafeText = true
		reader.typedRune = reader.typedRune[:0]
		return
	}
	reader.queued = append(reader.queued, reader.typedRune[:size]...)
	reader.typedRune = reader.typedRune[:0]
}

func (reader *bracketedPasteReader) finishTypedRune() {
	if len(reader.typedRune) == 0 {
		return
	}
	reader.invalidUTF8 = true
	reader.typedRune = reader.typedRune[:0]
}

func (reader *bracketedPasteReader) consumePasteByte(value byte) {
	reader.lineBytes++
	if reader.lineBytes > reader.maxBytes {
		reader.overflow = true
		return
	}
	reader.rawPaste = append(reader.rawPaste, value)
	reader.pasteRune = append(reader.pasteRune, value)
	if !utf8.FullRune(reader.pasteRune) {
		return
	}
	character, size := utf8.DecodeRune(reader.pasteRune)
	if character == utf8.RuneError && size == 1 {
		reader.invalidUTF8 = true
		reader.pasteRune = reader.pasteRune[:0]
		return
	}
	encoded := reader.pasteRune[:size]
	reader.pasteRune = reader.pasteRune[size:]
	if unsafeTerminalTextRune(character) && character != '\r' &&
		character != '\n' && character != '\t' {
		reader.unsafeText = true
		return
	}
	if character == '\r' {
		reader.queued = append(reader.queued, '\n')
		reader.pasteCR = true
		return
	}
	if character == '\n' && reader.pasteCR {
		reader.pasteCR = false
		return
	}
	reader.pasteCR = false
	reader.queued = append(reader.queued, encoded...)
}

func (reader *bracketedPasteReader) finishPastePayload() {
	if len(reader.pasteRune) > 0 {
		reader.invalidUTF8 = true
		reader.pasteRune = reader.pasteRune[:0]
	}
}

func (reader *bracketedPasteReader) consumeLineState() (bool, bool, bool, bool, bool, string, terminalInputAuthority) {
	reader.finishTypedRune()
	pasted := reader.linePasted
	overflow := reader.overflow
	mixed := reader.lineMixed
	invalidUTF8 := reader.invalidUTF8
	unsafeText := reader.unsafeText
	exactPaste := string(reader.rawPaste)
	authority := reader.lineAuthority
	reader.linePasted = false
	reader.lineMixed = false
	reader.lineBytes = 0
	reader.overflow = false
	reader.invalidUTF8 = false
	reader.unsafeText = false
	reader.rawPaste = reader.rawPaste[:0]
	reader.pasteRune = reader.pasteRune[:0]
	reader.typedRune = reader.typedRune[:0]
	reader.lineAuthority = terminalInputAuthority{}
	reader.lineAuthoritySet = false
	return pasted, overflow, mixed, invalidUTF8, unsafeText, exactPaste, authority
}

func (reader *bracketedPasteReader) mergeLineAuthority(authority terminalInputAuthority) {
	if !reader.lineAuthoritySet {
		reader.lineAuthority = authority
		reader.lineAuthoritySet = true
		return
	}
	if reader.lineAuthority != authority {
		reader.lineAuthority = terminalInputAuthority{
			Generation: authority.Generation,
			Mode:       terminalInputMixed,
		}
	}
}
