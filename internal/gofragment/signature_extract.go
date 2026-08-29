package gofragment

import (
	"errors"
	"fmt"
	"go/scanner"
	"go/token"
	"strings"
	"unicode/utf8"
)

const maxGoSignatureAuthorityBytes = 4096

var ErrNoExplicitFunctionSignature = errors.New("objective contains no explicit Go function signature")

// ExtractUniqueNewFunctionSignature recognizes explicit Go syntax inside one
// bounded authority. It is a syntax parser, not natural-language routing: no
// phrasing, verbs, paths, or filenames influence the result.
func ExtractUniqueNewFunctionSignature(authority string) (NewFunctionSignature, error) {
	if authority == "" || len(authority) > maxGoSignatureAuthorityBytes || !utf8.ValidString(authority) || strings.IndexByte(authority, 0) >= 0 {
		return NewFunctionSignature{}, fmt.Errorf("Go signature authority must be valid UTF-8 within %d bytes", maxGoSignatureAuthorityBytes)
	}
	file := token.NewFileSet().AddFile("", -1, len(authority))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(authority), nil, scanner.ScanComments)
	starts := make([]int, 0, 2)
	for {
		position, current, _ := lexer.Scan()
		if current == token.EOF {
			break
		}
		if current == token.FUNC {
			starts = append(starts, file.Offset(position))
		}
	}
	if len(starts) == 0 {
		return NewFunctionSignature{}, ErrNoExplicitFunctionSignature
	}
	if len(starts) != 1 {
		return NewFunctionSignature{}, fmt.Errorf("objective contains %d explicit Go function signature candidates", len(starts))
	}
	start := starts[0]
	var best NewFunctionSignature
	bestBytes := 0
	bestEnd := 0
	for end := start + len("func"); end <= len(authority); end++ {
		candidate := strings.TrimSpace(authority[start:end])
		if candidate == "" || strings.ContainsAny(candidate, "\r\n") {
			continue
		}
		compiled, err := CompileNewFunctionSignature(candidate)
		if err != nil {
			continue
		}
		if len(candidate) > bestBytes {
			best, bestBytes, bestEnd = compiled, len(candidate), end
		}
	}
	if bestBytes == 0 {
		return NewFunctionSignature{}, fmt.Errorf("objective has no complete supported Go function signature")
	}
	best.Source = strings.TrimSpace(authority[start:bestEnd])
	best.StartByte = start
	best.EndByte = bestEnd
	return best, nil
}
