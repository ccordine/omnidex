package modelcontext

import (
	"strconv"
	"unicode/utf8"
)

type sourceLiteralByteOrigin struct {
	start int
	end   int
}

// appendDecodedSourceLiteralPathIdentities inspects the semantic bytes of an
// interpreted source literal instead of treating its escape introducers as
// filesystem separators. Every decoded byte retains the exact raw span that
// produced it, so a rejected identity is still reported against source bytes.
func appendDecodedSourceLiteralPathIdentities(
	destination []PathIdentity,
	raw string,
	offset int,
	provenance ArtifactIdentityProvenance,
) []PathIdentity {
	view, origins := decodedSourceLiteralPathView(raw)
	for _, identity := range PathIdentities(view, provenance) {
		if identity.Start < 0 || identity.End <= identity.Start || identity.End > len(origins) {
			continue
		}
		viewStart, viewEnd := identity.Start, identity.End
		rawStart := origins[viewStart].start
		rawEnd := origins[viewEnd-1].end
		if _, proven := provenance.resolve(view[viewStart:viewEnd]); !proven {
			identity.Value = raw[rawStart:rawEnd]
		}
		identity.Start = offset + rawStart
		identity.End = offset + rawEnd
		destination = append(destination, identity)
	}
	return destination
}

func decodedSourceLiteralPathView(raw string) (string, []sourceLiteralByteOrigin) {
	view := make([]byte, 0, len(raw))
	origins := make([]sourceLiteralByteOrigin, 0, len(raw))
	for cursor := 0; cursor < len(raw); {
		if raw[cursor] == '\\' {
			decoded, end, ok := decodeSourceLiteralEscape(raw, cursor)
			if ok {
				for _, value := range []byte(decoded) {
					view = append(view, value)
					origins = append(origins, sourceLiteralByteOrigin{start: cursor, end: end})
				}
				cursor = end
				continue
			}
		}
		view = append(view, raw[cursor])
		origins = append(origins, sourceLiteralByteOrigin{start: cursor, end: cursor + 1})
		cursor++
	}
	return string(view), origins
}

func decodeSourceLiteralEscape(raw string, start int) (string, int, bool) {
	if start+1 >= len(raw) || raw[start] != '\\' {
		return "", start, false
	}
	next := raw[start+1]
	if decoded, exists := sourceSimpleEscape(next); exists {
		return decoded, start + 2, true
	}
	if next == '\n' {
		return "\n", start + 2, true
	}
	if next == '\r' {
		end := start + 2
		if end < len(raw) && raw[end] == '\n' {
			end++
		}
		return "\n", end, true
	}
	if next >= '0' && next <= '7' {
		end := start + 2
		for end < len(raw) && end < start+4 && raw[end] >= '0' && raw[end] <= '7' {
			end++
		}
		return decodeSourceEscapeCodePoint(raw[start+1:end], 8, end)
	}
	if next == 'x' && start+4 <= len(raw) && sourceHexBytes(raw[start+2:start+4]) {
		return decodeSourceEscapeCodePoint(raw[start+2:start+4], 16, start+4)
	}
	if next == 'u' {
		return decodeSourceUnicodeEscape(raw, start)
	}
	if next == 'U' && start+10 <= len(raw) && sourceHexBytes(raw[start+2:start+10]) {
		return decodeSourceEscapeCodePoint(raw[start+2:start+10], 16, start+10)
	}
	return "", start, false
}

func sourceSimpleEscape(value byte) (string, bool) {
	switch value {
	case '\\':
		return `\`, true
	case '/':
		return "/", true
	case '\'', '"', '`':
		return string(value), true
	case 'a':
		return "\a", true
	case 'b':
		return "\b", true
	case 'e':
		return "\x1b", true
	case 'f':
		return "\f", true
	case 'n':
		return "\n", true
	case 'r':
		return "\r", true
	case 't':
		return "\t", true
	case 'v':
		return "\v", true
	default:
		return "", false
	}
}

func decodeSourceUnicodeEscape(raw string, start int) (string, int, bool) {
	digitsStart := start + 2
	if digitsStart < len(raw) && raw[digitsStart] == '{' {
		digitsStart++
		end := digitsStart
		for end < len(raw) && end-digitsStart < 6 && sourceHexByte(raw[end]) {
			end++
		}
		if end == digitsStart || end >= len(raw) || raw[end] != '}' {
			return "", start, false
		}
		return decodeSourceEscapeCodePoint(raw[digitsStart:end], 16, end+1)
	}
	end := digitsStart + 4
	if end > len(raw) || !sourceHexBytes(raw[digitsStart:end]) {
		return "", start, false
	}
	return decodeSourceEscapeCodePoint(raw[digitsStart:end], 16, end)
}

func decodeSourceEscapeCodePoint(
	digits string,
	base int,
	end int,
) (string, int, bool) {
	value, err := strconv.ParseUint(digits, base, 32)
	if err != nil || !utf8.ValidRune(rune(value)) {
		return "", end, false
	}
	return string(rune(value)), end, true
}

func sourceHexBytes(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if !sourceHexByte(value[index]) {
			return false
		}
	}
	return true
}

func sourceHexByte(value byte) bool {
	return value >= '0' && value <= '9' ||
		value >= 'a' && value <= 'f' ||
		value >= 'A' && value <= 'F'
}
