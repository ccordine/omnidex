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
// The line editor treats CR and LF as submission even during bracketed paste.
// It receives a reversible single-line projection. The edited terminal line
// owns the complete text, including typed prefixes, suffixes and corrections.
type bracketedPasteReader struct {
	source           io.Reader
	paste            bool
	linePasted       bool
	invalidUTF8      bool
	unsafeText       bool
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

func newBracketedPasteReader(source io.Reader) *bracketedPasteReader {
	return &bracketedPasteReader{source: source}
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
		reader.queued = append(reader.queued, reader.candidate...)
		reader.candidate = reader.candidate[:0]
		reader.paste = !reader.paste
		if reader.paste {
			reader.linePasted = true
		}
	}
	lineEnd := !reader.paste && (value == '\r' || value == '\n') && len(reader.candidate) == 0
	if lineEnd {
		reader.skipLineLF = value == '\r'
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
	reader.pasteRune = reader.pasteRune[size:]
	if unsafeTerminalTextRune(character) && character != '\r' &&
		character != '\n' && character != '\t' {
		reader.unsafeText = true
		return
	}
	reader.queued = utf8.AppendRune(reader.queued, projectPasteRune(character))
}

func (reader *bracketedPasteReader) finishPastePayload() {
	if len(reader.pasteRune) > 0 {
		reader.invalidUTF8 = true
		reader.pasteRune = reader.pasteRune[:0]
	}
}

func (reader *bracketedPasteReader) consumeLineState() (bool, bool, bool, terminalInputAuthority) {
	reader.finishTypedRune()
	pasted := reader.linePasted
	invalidUTF8 := reader.invalidUTF8
	unsafeText := reader.unsafeText
	authority := reader.lineAuthority
	reader.linePasted = false
	reader.invalidUTF8 = false
	reader.unsafeText = false
	reader.pasteRune = reader.pasteRune[:0]
	reader.typedRune = reader.typedRune[:0]
	reader.lineAuthority = terminalInputAuthority{}
	reader.lineAuthoritySet = false
	return pasted, invalidUTF8, unsafeText, authority
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
